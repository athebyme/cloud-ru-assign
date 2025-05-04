//go:generate mockgen -source=driven.go -destination=../../test/mocks/driven_mock.go -package=mocks
package ports

import (
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"net/http"
)

// LoadBalancerService определяет основной входящий порт для обработки запросов
// это интерфейс, который ядро предоставляет внешнмм пингам
type LoadBalancerService interface {
	HandleRequest(w http.ResponseWriter, r *http.Request)
}

// RateLimitService определяет входящий порт для управления rate limiting
type RateLimitService interface {
	CreateOrUpdateClient(settings *ratelimit.RateLimitSettings) error
	RemoveClient(clientID string) error
	GetClientSettings(clientID string) (*ratelimit.RateLimitSettings, error)
	ListClients() ([]*ratelimit.RateLimitSettings, error)
}
