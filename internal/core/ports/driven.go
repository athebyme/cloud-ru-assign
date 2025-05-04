//go:generate mockgen -source=driven.go -destination=../../test/mocks/driven_mock.go -package=mocks
package ports

import (
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"net/http"
	"net/url"
)

// Logger определяет исходящий порт для логирования
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// HealthChecker определяет исходящий порт для проверки состояния бэкенда
type HealthChecker interface {
	Check(target *url.URL) error
}

// BackendRepository определяет исходящий порт для управления состоянием и выбором бэкендов
type BackendRepository interface {
	GetBackends() []*balancer.Backend
	MarkBackendStatus(backendUrl *url.URL, alive bool)
	GetNextHealthyBackend() (*balancer.Backend, bool)
	SetStrategy(strategy string) error
	GetActiveConnections(backend *balancer.Backend) int
	IncrementConnections(backend *balancer.Backend)
	DecrementConnections(backend *balancer.Backend)
}

// Forwarder определяет исходящий порт для пересылки (проксирования) запроса на бэкенд
type Forwarder interface {
	// Forward проксирует входящий запрос r на целевой бэкенд target, используя w для ответа
	// возвращает ошибку, если операция проксирования не удалась
	Forward(w http.ResponseWriter, r *http.Request, target *balancer.Backend) error
}

// RateLimiter определяет исходящий порт для проверки ограничений скорости
type RateLimiter interface {
	Allow(clientID string) bool
	SetRateLimit(clientID string, settings *ratelimit.RateLimitSettings) error
	RemoveRateLimit(clientID string) error
	Stop()
}
