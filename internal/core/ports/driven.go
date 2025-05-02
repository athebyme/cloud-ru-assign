package ports

import (
	"cloud-ru-assign/internal/core/domain"
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
	GetBackends() []*domain.Backend
	MarkBackendStatus(backendUrl *url.URL, alive bool)
	GetNextHealthyBackend() (*domain.Backend, bool)
}

// Forwarder определяет исходящий порт для пересылки (проксирования) запроса на бэкенд
type Forwarder interface {
	// Forward проксирует входящий запрос r на целевой бэкенд target, используя w для ответа
	// возвращает ошибку, если операция проксирования не удалась
	Forward(w http.ResponseWriter, r *http.Request, target *domain.Backend) error
}
