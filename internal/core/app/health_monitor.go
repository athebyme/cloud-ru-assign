package app

import (
	"context"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"sync"
	"time"
)

// HealthMonitor периодически проверяет состояние бэкендов
type HealthMonitor struct {
	updater  ports.BackendRepository // интерфейс для обновления статуса бэкендов (реализован репозиторием)
	checker  ports.HealthChecker     // интерфейс для выполнения проверки (например, HTTP)
	logger   ports.Logger
	interval time.Duration
	stopCh   chan struct{} // канал для сигнала остановки мониторинга
	wg       sync.WaitGroup
}

// NewHealthMonitor создает новый монитор состояния
func NewHealthMonitor(
	updater ports.BackendRepository,
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

// Start запускает горутину для периодической проверки состояния
func (hm *HealthMonitor) Start() {
	hm.logger.Info("Запуск мониторинга состояния бэкендов", "interval", hm.interval)
	hm.wg.Add(1)

	go func() {
		defer hm.wg.Done()
		ticker := time.NewTicker(hm.interval)
		defer ticker.Stop()

		hm.performChecks() // выполняем проверку сразу при старте

		// основной цикл мониторинга
		for {
			select {
			case <-ticker.C:
				hm.performChecks() // выполняем проверки по тикеру
			case <-hm.stopCh:
				hm.logger.Info("Остановка мониторинга состояния бэкендов")
				return // выходим из горутины если получили stop сигнал
			}
		}
	}()
}

// performChecks получает список бэкендов и проверяет их состояние конкурентно
func (hm *HealthMonitor) performChecks() {
	backends := hm.updater.GetBackends()
	hm.logger.Debug("Начало цикла проверки состояния", "backend_count", len(backends))

	var checkWg sync.WaitGroup
	for _, backend := range backends {
		checkWg.Add(1)

		// запускаем проверку каждого бэкенда в отдельной горутине
		go func(b *balancer.Backend) {
			defer checkWg.Done()
			checkLogger := hm.logger.With("backend_url", b.URL.String()) // логгер с контекстом бэкенда

			// выполняем проверку через checker
			err := hm.checker.Check(b.URL)
			isAlive := err == nil // здоров, если ошибки нет

			if isAlive {
				checkLogger.Debug("Бэкенд доступен (health check OK)")
			} else {
				checkLogger.Warn("Бэкенд недоступен (health check Failed)", "error", err)
			}

			// обновляем статус бэкенда через updater (репозиторий)
			hm.updater.MarkBackendStatus(b.URL, isAlive)

		}(backend)
	}

	checkWg.Wait()
	hm.logger.Debug("Завершение цикла проверки состояния")
}

// Stop останавливает мониторинг и дожидается завершения
func (hm *HealthMonitor) Stop(ctx context.Context) {
	hm.logger.Info("Сигнал остановки для Health Monitor")
	close(hm.stopCh) // посылаем сигнал остановки в горутину

	waitCh := make(chan struct{})
	go func() {
		hm.wg.Wait() // ждем, пока wg.Done() не будет вызван в горутине
		close(waitCh)
	}()

	select {
	case <-waitCh:
		hm.logger.Info("Health Monitor остановлен")
	case <-ctx.Done(): // если сработал таймаут ожидания
		hm.logger.Warn("Таймаут ожидания остановки Health Monitor", "error", ctx.Err())
	}
}
