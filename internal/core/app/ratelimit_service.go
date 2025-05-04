package app

import (
	"errors"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"sync"
)

// rateLimitService реализует входящий порт RateLimitService
type rateLimitService struct {
	limiter  ports.RateLimiter
	settings map[string]*ratelimit.RateLimitSettings
	mu       sync.RWMutex
	logger   ports.Logger
}

// NewRateLimitService создает новый сервис управления rate limiting
func NewRateLimitService(limiter ports.RateLimiter, logger ports.Logger) ports.RateLimitService {
	return &rateLimitService{
		limiter:  limiter,
		settings: make(map[string]*ratelimit.RateLimitSettings),
		logger:   logger.With("service", "RateLimitService"),
	}
}

// CreateOrUpdateClient создает или обновляет настройки клиента
func (s *rateLimitService) CreateOrUpdateClient(settings *ratelimit.RateLimitSettings) error {
	if settings.ClientID == "" {
		return errors.New("client_id не может быть пустым")
	}
	if settings.Capacity <= 0 {
		return errors.New("capacity должно быть больше 0")
	}
	if settings.RatePerSecond <= 0 {
		return errors.New("rate_per_second должно быть больше 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings[settings.ClientID] = settings

	err := s.limiter.SetRateLimit(settings.ClientID, settings)
	if err != nil {
		return err
	}

	s.logger.Info("Rate limit settings updated", "client", settings.ClientID)
	return nil
}

// RemoveClient удаляет клиента
func (s *rateLimitService) RemoveClient(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.settings[clientID]; !exists {
		return errors.New("client not found")
	}

	delete(s.settings, clientID)
	s.limiter.RemoveRateLimit(clientID)

	s.logger.Info("Client removed", "client", clientID)
	return nil
}

// GetClientSettings получает настройки клиента
func (s *rateLimitService) GetClientSettings(clientID string) (*ratelimit.RateLimitSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	settings, exists := s.settings[clientID]
	if !exists {
		return nil, errors.New("client not found")
	}

	return settings, nil
}

// ListClients возвращает список всех клиентов
func (s *rateLimitService) ListClients() ([]*ratelimit.RateLimitSettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clients := make([]*ratelimit.RateLimitSettings, 0, len(s.settings))
	for _, settings := range s.settings {
		clients = append(clients, settings)
	}

	return clients, nil
}
