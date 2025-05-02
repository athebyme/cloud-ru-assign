package ports

import (
	"cloud-ru-assign/internal/core/domain"
	"net/http"
	"net/url"
)

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

type HealthChecker interface {
	Check(target *url.URL) error
}

type BackendStatusUpdater interface {
	MarkBackendStatus(backendUrl *url.URL, alive bool)
	GetBackends() []*domain.Backend
}

type BackendRepository interface {
	GetBackends() []*domain.Backend
	MarkBackendStatus(backendUrl *url.URL, alive bool)
	GetNextHealthyBackend() (*domain.Backend, bool)
}

type Forwarder interface {
	Forward(w http.ResponseWriter, r *http.Request, target *domain.Backend) error
}
