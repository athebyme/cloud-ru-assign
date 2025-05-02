package main

import (
	"cloud-ru-assign/internal/adapters/healthcheck"
	"cloud-ru-assign/internal/adapters/http"
	logadapter "cloud-ru-assign/internal/adapters/log"
	"cloud-ru-assign/internal/adapters/pool"
	"cloud-ru-assign/internal/adapters/proxy"
	"cloud-ru-assign/internal/config"
	"cloud-ru-assign/internal/core/app"
	"context"
	"flag"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// TODO: Error var`s

func main() {
	// --- парсинг флагов и загрузка конфигурации ---
	configPath := flag.String("config", "./configs/config.yml", "Path to YAML config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		// используем базовый логгер для критических ошибок старта
		bootstrapLogger := logadapter.NewSlogAdapter("error", false)
		bootstrapLogger.Error("не удалось загрузить конфигурацию", "error", err, "path", *configPath)
		os.Exit(1)
	}

	// --- инициализация логгера ---
	logger := logadapter.NewSlogAdapter(cfg.Log.Level, cfg.Log.Format == "json")
	logger.Info("конфигурация успешно загружена", "config", cfg)

	// --- Dependency Injection ---
	// 1 инициализируем исходящие адаптеры (реализации driven ports)
	backendRepo, err := pool.NewMemoryPool(cfg.Backends, logger)
	if err != nil {
		logger.Error("не удалось создать репозиторий бэкендов", "error", err)
		os.Exit(1)
	}
	forwarder := proxy.NewHttpUtilForwarder(logger)
	checker := healthcheck.NewHTTPChecker(cfg.HealthCheck.Timeout, cfg.HealthCheck.Path)

	// 2 инициализируем сервисы приложения (реализации use cases)
	lbService := app.NewLoadBalancerService(backendRepo, forwarder, logger)
	var healthMonitor *app.HealthMonitor
	if cfg.HealthCheck.Enabled {
		healthMonitor = app.NewHealthMonitor(backendRepo, checker, logger, cfg.HealthCheck.Interval)
	}

	// 3 инициализируем входящие адаптеры (реализации driving ports)
	httpAdapter := http.NewServerAdapter(cfg.ListenAddress, lbService, logger)

	// --- Запуск компонентов приложения ---
	var wg sync.WaitGroup

	// запускаем монитор состояния (если включен)
	if healthMonitor != nil {
		healthMonitor.Start() // запускается в фоне - не блокирующий
		logger.Info("монитор состояния запущен")
	}

	httpAdapter.Run()

	// --- Обработка сигналов для Graceful Shutdown ---

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("получен сигнал завершения работы", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// --- Последовательность остановки компонентов ---

	// сначала останавливаем монитор (если он был запущен)
	if healthMonitor != nil {
		wg.Add(1)
		go func() { // запускаем остановку в горутине, чтобы не блокировать основной поток
			defer wg.Done()

			// даем монитору свой таймаут на остановку
			monitorCtx, monitorCancel := context.WithTimeout(shutdownCtx, 4*time.Second)
			defer monitorCancel()
			healthMonitor.Stop(monitorCtx)
		}()
	}

	// затем останавливаем HTTP сервер
	wg.Add(1)
	go func() { // также в горутине
		defer wg.Done()
		// даем серверу свой таймаут
		serverCtx, serverCancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer serverCancel()
		httpAdapter.Stop(serverCtx)
	}()

	// ждем завершения всех остановок
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("все компоненты успешно остановлены")
	case <-shutdownCtx.Done(): // если истек общий таймаут на остановку
		logger.Error("общий таймаут остановки приложения", "error", shutdownCtx.Err())
	}

	logger.Info("приложение завершило работу")
}
