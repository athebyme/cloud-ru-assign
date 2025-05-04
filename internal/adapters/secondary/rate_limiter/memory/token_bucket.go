package memory

import (
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"sync"
	"time"
)

// TokenBucket представляет ведро токенов для одного клиента
type TokenBucket struct {
	tokens     int64      // текущее кол-во токенов
	capacity   int64      // максимальная емкость
	refillRate int64      // кол-во токенов в секунду
	lastRefill time.Time  // время последнего пополнения
	mu         sync.Mutex // для потокобезопасности
}

// ClientState сочетает ведро токенов с настройками клиента
type ClientState struct {
	bucket   *TokenBucket
	settings *ratelimit.RateLimitSettings
}

// MemoryRateLimiter реализует порт RateLimiter using in-memory storage
type MemoryRateLimiter struct {
	clients map[string]*ClientState // ключ: IP или API ключ
	mu      sync.RWMutex
	logger  ports.Logger
	ticker  *time.Ticker
	stopCh  chan struct{}
}

// NewMemoryRateLimiter создает новый rate limiter
func NewMemoryRateLimiter(logger ports.Logger) *MemoryRateLimiter {
	rl := &MemoryRateLimiter{
		clients: make(map[string]*ClientState),
		logger:  logger.With("component", "RateLimiter"),
		ticker:  time.NewTicker(time.Second),
		stopCh:  make(chan struct{}),
	}

	go rl.refillLoop()

	return rl
}

// Allow проверяет, может ли клиент сделать запрос
func (rl *MemoryRateLimiter) Allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	state, exists := rl.clients[clientID]
	if !exists {
		return true
	}

	bucket := state.bucket
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	if bucket.tokens > 0 {
		bucket.tokens--
		rl.logger.Debug("Token consumed", "client", clientID, "tokens_left", bucket.tokens)
		return true
	}

	rl.logger.Debug("Rate limit exceeded", "client", clientID)
	return false
}

// SetRateLimit устанавливает настройки для клиента
func (rl *MemoryRateLimiter) SetRateLimit(clientID string, settings *ratelimit.RateLimitSettings) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket := &TokenBucket{
		tokens:     settings.Capacity,
		capacity:   settings.Capacity,
		refillRate: settings.RatePerSecond,
		lastRefill: time.Now(),
	}

	rl.clients[clientID] = &ClientState{
		bucket:   bucket,
		settings: settings,
	}

	rl.logger.Info("Rate limit set for client", "client", clientID, "capacity", settings.Capacity, "rate", settings.RatePerSecond)

	return nil
}

// RemoveRateLimit удаляет ограничения для клиента
func (rl *MemoryRateLimiter) RemoveRateLimit(clientID string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.clients, clientID)
	rl.logger.Info("Rate limit removed for client", "client", clientID)
	return nil
}

// refillLoop пополняет токены для всех клиентов
func (rl *MemoryRateLimiter) refillLoop() {
	for {
		select {
		case <-rl.ticker.C:
			rl.refillAll()
		case <-rl.stopCh:
			return
		}
	}
}

// refillAll пополняет токены для всех клиентов
func (rl *MemoryRateLimiter) refillAll() {
	rl.mu.RLock()
	for clientID, state := range rl.clients {
		state.bucket.mu.Lock()

		now := time.Now()
		elapsed := now.Sub(state.bucket.lastRefill).Seconds()
		tokensToAdd := int64(elapsed * float64(state.bucket.refillRate))

		if tokensToAdd > 0 {
			state.bucket.tokens = minimum(state.bucket.capacity, state.bucket.tokens+tokensToAdd)
			state.bucket.lastRefill = now
			rl.logger.Debug("Tokens refilled", "client", clientID, "tokens", state.bucket.tokens)
		}

		state.bucket.mu.Unlock()
	}
	rl.mu.RUnlock()
}

// Stop останавливает rate limiter
func (rl *MemoryRateLimiter) Stop() {
	close(rl.stopCh)
	rl.ticker.Stop()
	rl.logger.Info("Rate limiter stopped")
}

func minimum(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
