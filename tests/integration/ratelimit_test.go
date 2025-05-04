package integration

import (
	"sync"
	"testing"
	"time"

	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/logger"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/rate_limiter/memory"
	"github.com/athebyme/cloud-ru-assign/internal/core/app"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
)

func TestRateLimiter_TokenBucket(t *testing.T) {
	logger := logger.NewSlogAdapter("error", false)
	rateLimiter := memory.NewMemoryRateLimiter(logger)
	defer rateLimiter.Stop()

	clientID := "test-client"
	settings := &ratelimit.RateLimitSettings{
		ClientID:      clientID,
		Capacity:      5,
		RatePerSecond: 1,
	}

	err := rateLimiter.SetRateLimit(clientID, settings)
	if err != nil {
		t.Fatalf("Failed to set rate limit: %v", err)
	}

	// допускается 5 запросов сразу
	for i := 0; i < 5; i++ {
		if !rateLimiter.Allow(clientID) {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// 6-й запрос должен быть блокирован
	if rateLimiter.Allow(clientID) {
		t.Error("6th request should be blocked")
	}

	// ждем пополнения токенов
	time.Sleep(2 * time.Second)

	// после пополнения опять должен быть разрешен
	if !rateLimiter.Allow(clientID) {
		t.Error("Request after refill should be allowed")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	logger := logger.NewSlogAdapter("error", false)
	rateLimiter := memory.NewMemoryRateLimiter(logger)
	defer rateLimiter.Stop()

	clientID := "concurrent-client"
	settings := &ratelimit.RateLimitSettings{
		ClientID:      clientID,
		Capacity:      100,
		RatePerSecond: 10,
	}

	err := rateLimiter.SetRateLimit(clientID, settings)
	if err != nil {
		t.Fatalf("Failed to set rate limit: %v", err)
	}

	const goroutines = 50
	const requestsPerGoroutine = 5

	var wg sync.WaitGroup
	allowedCount := 0
	blockedCount := 0
	var mu sync.Mutex

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				mu.Lock()
				if rateLimiter.Allow(clientID) {
					allowedCount++
				} else {
					blockedCount++
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// должно быть разрешено ровно 100 запросов
	if allowedCount != 100 {
		t.Errorf("Expected 100 allowed requests, got %d", allowedCount)
	}
	if blockedCount != 150 {
		t.Errorf("Expected 150 blocked requests, got %d", blockedCount)
	}
}

func TestRateLimiter_DifferentClients(t *testing.T) {
	logger := logger.NewSlogAdapter("error", false)
	rateLimiter := memory.NewMemoryRateLimiter(logger)
	defer rateLimiter.Stop()

	// разные настройки для разных клиентов
	clients := []struct {
		id       string
		capacity int64
		rate     int64
	}{
		{"client1", 5, 1},
		{"client2", 10, 2},
		{"client3", 3, 1},
	}

	// устанавливаем лимиты
	for _, client := range clients {
		settings := &ratelimit.RateLimitSettings{
			ClientID:      client.id,
			Capacity:      client.capacity,
			RatePerSecond: client.rate,
		}
		rateLimiter.SetRateLimit(client.id, settings)
	}

	// тестируем каждого клиента
	for _, client := range clients {
		allowedCount := 0
		for {
			if rateLimiter.Allow(client.id) {
				allowedCount++
			} else {
				break
			}
		}

		if int64(allowedCount) != client.capacity {
			t.Errorf("Client %s: expected %d allowed requests, got %d",
				client.id, client.capacity, allowedCount)
		}
	}
}

func TestRateLimit_RoundRobinAPI(t *testing.T) {
	logger := logger.NewSlogAdapter("error", false)
	limiter := memory.NewMemoryRateLimiter(logger)
	defer limiter.Stop()

	service := app.NewRateLimitService(limiter, logger)

	// создаем нескольких клиентов
	clients := []string{"client-a", "client-b", "client-c"}
	for _, clientID := range clients {
		settings := &ratelimit.RateLimitSettings{
			ClientID:      clientID,
			Capacity:      100,
			RatePerSecond: 10,
		}
		err := service.CreateOrUpdateClient(settings)
		if err != nil {
			t.Fatalf("Failed to create client %s: %v", clientID, err)
		}
	}

	// получаем список всех клиентов
	allClients, err := service.ListClients()
	if err != nil {
		t.Fatalf("Failed to list clients: %v", err)
	}

	if len(allClients) != len(clients) {
		t.Errorf("Expected %d clients, got %d", len(clients), len(allClients))
	}

	// обновляем настройки одного клиента
	updateSettings := &ratelimit.RateLimitSettings{
		ClientID:      "client-a",
		Capacity:      200,
		RatePerSecond: 20,
	}
	err = service.CreateOrUpdateClient(updateSettings)
	if err != nil {
		t.Fatalf("Failed to update client: %v", err)
	}

	// проверяем обновление
	updatedSettings, err := service.GetClientSettings("client-a")
	if err != nil {
		t.Fatalf("Failed to get updated settings: %v", err)
	}

	if updatedSettings.Capacity != 200 || updatedSettings.RatePerSecond != 20 {
		t.Error("Client settings were not updated properly")
	}

	// удаляем клиента
	err = service.RemoveClient("client-b")
	if err != nil {
		t.Fatalf("Failed to remove client: %v", err)
	}

	// проверяем, что клиент удален
	allClients, _ = service.ListClients()
	if len(allClients) != len(clients)-1 {
		t.Errorf("Expected %d clients after removal, got %d", len(clients)-1, len(allClients))
	}
}
