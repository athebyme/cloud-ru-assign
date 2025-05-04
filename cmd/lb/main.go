package main

import (
	"context"
	"flag"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/load_balancer/healthcheck"
	logadapter "github.com/athebyme/cloud-ru-assign/internal/adapters/load_balancer/log"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/load_balancer/pool"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/load_balancer/proxy"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/rate_limiter/middleware"
	"github.com/athebyme/cloud-ru-assign/internal/config"
	"github.com/athebyme/cloud-ru-assign/internal/core/app"
	"net/http"
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
		bootstrapLogger := logadapter.NewSlogAdapter("error", false)
		bootstrapLogger.Error("не удалось загрузить конфигурацию", "error", err, "path", *configPath)
		os.Exit(1)
	}

	// --- инициализация логгера ---
	logger := logadapter.NewSlogAdapter(cfg.Log.Level, cfg.Log.Format == "json")
	logger.Info("конфигурация успешно загружена", "config", cfg)

	// --- Dependency Injection ---
	// 1 инициализируем исходящие адаптеры
	backendRepo, err := pool.NewMemoryPool(cfg.Backends, logger)
	if err != nil {
		logger.Error("не удалось создать репозиторий бэкендов", "error", err)
		os.Exit(1)
	}
	forwarder := proxy.NewHttpUtilForwarder(logger)
	checker := healthcheck.NewHTTPChecker(cfg.HealthCheck.Timeout, cfg.HealthCheck.Path)

	// 2 инициализируем rate limiter
	var rateLimiter *ratelimit.MemoryRateLimiter
	var rateLimitService app.RateLimitService
	if cfg.RateLimit.Enabled {
		rateLimiter = ratelimit.NewMemoryRateLimiter(logger)
		rateLimitService = app.NewRateLimitService(rateLimiter, logger)
	}

	// 3 инициализируем сервисы приложения
	lbService := app.NewLoadBalancerService(backendRepo, forwarder, logger)
	var healthMonitor *app.HealthMonitor
	if cfg.HealthCheck.Enabled {
		healthMonitor = app.NewHealthMonitor(backendRepo, checker, logger, cfg.HealthCheck.Interval)
	}

	// 4 инициализируем HTTP сервер с middleware
	mux := http.NewServeMux()
	if cfg.RateLimit.Enabled && cfg.RateLimit.Middleware {
		// Оборачиваем lbService.HandleRequest в middleware
		handler := http.HandlerFunc(lbService.HandleRequest)
		rateLimitedHandler := middleware.RateLimitMiddleware(rateLimiter, logger)(handler)
		mux.Handle("/", rateLimitedHandler)

		// Добавляем API для управления rate limiting
		if rateLimitService != nil {
			apiHandler := http.NewRateLimitAPIHandler(rateLimitService, logger)
			apiHandler.RegisterRoutes(mux)
		}
	} else {
		mux.HandleFunc("/", lbService.HandleRequest)
	}

	httpAdapter := http.NewServerAdapter(cfg.ListenAddress, lbService, logger)
	httpAdapter.Server.Handler = mux

	// --- Запуск компонентов приложения ---
	var wg sync.WaitGroup

	if healthMonitor != nil {
		healthMonitor.Start()
		logger.Info("монитор состояния запущен")
	}

	httpAdapter.Run()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("получен сигнал завершения работы", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	// Останавливаем rate limiter
	if rateLimiter != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rateLimiter.Stop()
		}()
	}

	// Останавливаем health monitor
	if healthMonitor != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitorCtx, monitorCancel := context.WithTimeout(shutdownCtx, 4*time.Second)
			defer monitorCancel()
			healthMonitor.Stop(monitorCtx)
		}()
	}

	// Останавливаем HTTP сервер
	wg.Add(1)
	go func() {
		defer wg.Done()
		serverCtx, serverCancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer serverCancel()
		httpAdapter.Stop(serverCtx)
	}()

	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		logger.Info("все компоненты успешно остановлены")
	case <-shutdownCtx.Done():
		logger.Error("общий таймаут остановки приложения", "error", shutdownCtx.Err())
	}

	logger.Info("приложение завершило работу")
}
