package app

import (
	"cloud-ru-assign/internal/core/domain"
	"cloud-ru-assign/internal/core/ports"
	"context"
	"sync"
	"time"
)

type HealthMonitor struct {
	updater  ports.BackendStatusUpdater
	checker  ports.HealthChecker
	logger   ports.Logger
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewHealthMonitor(
	updater ports.BackendStatusUpdater,
	checker ports.HealthChecker,
	logger ports.Logger,
	interval time.Duration,
) *HealthMonitor {
	return &HealthMonitor{
		updater:  updater,
		checker:  checker,
		logger:   logger.With("component", "HealthMonitor"),
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (hm *HealthMonitor) Start() {
	hm.logger.Info("Запуск мониторинга состояния бэкендов", "interval", hm.interval)
	hm.wg.Add(1)

	go func() {
		defer hm.wg.Done()
		ticker := time.NewTicker(hm.interval)
		defer ticker.Stop()

		hm.performChecks()

		for {
			select {
			case <-ticker.C:
				hm.performChecks()
			case <-hm.stopCh:
				hm.logger.Info("Остановка мониторинга состояния бэкендов")
				return
			}
		}
	}()
}

func (hm *HealthMonitor) performChecks() {
	backends := hm.updater.GetBackends()
	hm.logger.Debug("Начало цикла проверки состояния", "backend_count", len(backends))

	var checkWg sync.WaitGroup
	for _, backend := range backends {
		checkWg.Add(1)
		go func(b *domain.Backend) {
			defer checkWg.Done()
			checkLogger := hm.logger.With("backend_url", b.URL.String())

			err := hm.checker.Check(b.URL)
			isAlive := err == nil

			if isAlive {
				checkLogger.Debug("Бэкенд доступен (health check OK)")
			} else {
				checkLogger.Warn("Бэкенд недоступен (health check Failed)", "error", err)
			}

			hm.updater.MarkBackendStatus(b.URL, isAlive)

		}(backend)
	}

	checkWg.Wait()
	hm.logger.Debug("Завершение цикла проверки состояния")
}

func (hm *HealthMonitor) Stop(ctx context.Context) {
	hm.logger.Info("Сигнал остановки для Health Monitor")
	close(hm.stopCh)

	waitCh := make(chan struct{})
	go func() {
		hm.wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		hm.logger.Info("Health Monitor остановлен")
	case <-ctx.Done():
		hm.logger.Warn("Таймаут ожидания остановки Health Monitor", "error", ctx.Err())
	}
}
