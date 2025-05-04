package main

import (
	"context"
	"flag"
	"fmt"
	ratelimit_http "github.com/athebyme/cloud-ru-assign/internal/adapters/primary/http"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/primary/http/middleware"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/healthcheck"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/logger"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/proxy"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/rate_limiter/memory"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/repository"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/storage/hybrid"
	"github.com/athebyme/cloud-ru-assign/internal/config"
	"github.com/athebyme/cloud-ru-assign/internal/core/app"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
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
		bootstrapLogger := logger.NewSlogAdapter("error", false)
		bootstrapLogger.Error("не удалось загрузить конфигурацию", "error", err, "path", *configPath)
		os.Exit(1)
	}

	// --- инициализация логгера ---
	slogAdapter := logger.NewSlogAdapter(cfg.Log.Level, cfg.Log.Format == "json")
	slogAdapter.Info("конфигурация успешно загружена", "config", cfg)

	// --- Dependency Injection ---
	// 1 инициализируем исходящие адаптеры
	backendRepo, err := repository.NewMemoryPool(cfg.Backends, slogAdapter)
	if err != nil {
		slogAdapter.Error("не удалось создать репозиторий бэкендов", "error", err)
		os.Exit(1)
	}
	forwarder := proxy.NewHttpUtilForwarder(slogAdapter)
	checker := healthcheck.NewHTTPChecker(cfg.HealthCheck.Timeout, cfg.HealthCheck.Path)

	// 2 инициализируем rate limiter
	var rateLimiter ports.RateLimiter
	var rateLimitService ports.RateLimitService
	if cfg.RateLimit.Enabled {
		storageType := os.Getenv("STORAGE_TYPE")

		switch storageType {
		case "hybrid":
			pgConnStr := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable",
				os.Getenv("POSTGRES_HOST"),
				os.Getenv("POSTGRES_USER"),
				os.Getenv("POSTGRES_PASSWORD"),
				os.Getenv("POSTGRES_DB"))

			redisAddr := os.Getenv("REDIS_ADDR")
			rateLimiter, err = hybrid.NewHybridRateLimiter(pgConnStr, redisAddr, slogAdapter)
			if err != nil {
				slogAdapter.Error("не удалось создать hybrid rate limiter", "error", err)
				os.Exit(1)
			}
		default:
			rateLimiter = memory.NewMemoryRateLimiter(slogAdapter)
		}

		rateLimitService = app.NewRateLimitService(rateLimiter, slogAdapter)
	}

	// 3 инициализируем сервисы приложения
	lbService := app.NewLoadBalancerService(backendRepo, forwarder, slogAdapter)
	var healthMonitor *app.HealthMonitor
	if cfg.HealthCheck.Enabled {
		healthMonitor = app.NewHealthMonitor(backendRepo, checker, slogAdapter, cfg.HealthCheck.Interval)
	}

	// 4 инициализируем HTTP сервер с middleware
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\n"))
	})

	if cfg.RateLimit.Enabled && cfg.RateLimit.Middleware {
		mainHandler := http.HandlerFunc(lbService.HandleRequest)
		rateLimitedMainHandler := middleware.RateLimitMiddleware(rateLimiter, slogAdapter)(mainHandler)

		if rateLimitService != nil {
			apiMux := http.NewServeMux()
			apiHandler := ratelimit_http.NewRateLimitAPIHandler(rateLimitService, slogAdapter)
			apiHandler.RegisterRoutes(apiMux)

			mux.Handle("/api/v1/ratelimit/", http.StripPrefix("/api/v1/ratelimit", apiMux))
			slogAdapter.Info("маршруты API с ограниченным тарифом, зарегистрированные в /api/v1/ratelimit/")
		}

		mux.Handle("/", rateLimitedMainHandler)

	} else {
		mux.HandleFunc("/", lbService.HandleRequest)
	}

	httpAdapter := ratelimit_http.NewServerAdapter(cfg.ListenAddress, lbService, slogAdapter)
	httpAdapter.Server.Handler = mux

	// --- Запуск компонентов приложения ---
	var wg sync.WaitGroup

	if healthMonitor != nil {
		healthMonitor.Start()
		slogAdapter.Info("монитор состояния запущен")
	}

	httpAdapter.Run()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slogAdapter.Info("получен сигнал завершения работы", "signal", sig.String())

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
		slogAdapter.Info("все компоненты успешно остановлены")
	case <-shutdownCtx.Done():
		slogAdapter.Error("общий таймаут остановки приложения", "error", shutdownCtx.Err())
	}

	slogAdapter.Info("приложение завершило работу")
}
