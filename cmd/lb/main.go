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
	configPath := flag.String("config", "./configs/config.yml", "Path to YAML config file")
	flag.Parse()
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		bootstrapLogger := logadapter.NewSlogAdapter("error", false)
		bootstrapLogger.Error("Failed to load configuration", "error", err, "path", *configPath)
		os.Exit(1)
	}
	logger := logadapter.NewSlogAdapter(cfg.Log.Level, cfg.Log.Format == "json")
	logger.Info("Configuration loaded", "config", cfg)

	backendRepo, err := pool.NewMemoryPool(cfg.Backends, logger)
	if err != nil {
		logger.Error("Failed to create backend repository", "error", err)
		os.Exit(1)
	}
	forwarder := proxy.NewHttpUtilForwarder(logger)
	checker := healthcheck.NewHTTPChecker(cfg.HealthCheck.Timeout, cfg.HealthCheck.Path)

	lbService := app.NewLoadBalancerService(backendRepo, forwarder, logger)
	var healthMonitor *app.HealthMonitor
	if cfg.HealthCheck.Enabled {
		healthMonitor = app.NewHealthMonitor(backendRepo, checker, logger, cfg.HealthCheck.Interval)
	}

	httpAdapter := http.NewServerAdapter(cfg.ListenAddress, lbService, logger)

	var wg sync.WaitGroup

	if healthMonitor != nil {
		healthMonitor.Start()
		logger.Info("Health monitor started")
	}

	httpAdapter.Run()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("Received shutdown signal", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if healthMonitor != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			monitorCtx, monitorCancel := context.WithTimeout(shutdownCtx, 4*time.Second)
			defer monitorCancel()
			healthMonitor.Stop(monitorCtx)
		}()
	}

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
		logger.Info("All components shut down gracefully.")
	case <-shutdownCtx.Done():
		logger.Error("Shutdown timed out.", "error", shutdownCtx.Err())
	}

	logger.Info("Application finished.")
}
