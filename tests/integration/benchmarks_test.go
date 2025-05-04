package integration

import (
	"fmt"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/logger"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/proxy"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/rate_limiter/memory"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/repository"
	"github.com/athebyme/cloud-ru-assign/internal/core/app"
)

func BenchmarkLoadBalancer_RoundRobin(b *testing.B) {
	// Создаем тестовые сервера
	testServers := []*httptest.Server{}
	for i := 0; i < 3; i++ {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		testServers = append(testServers, server)
		defer server.Close()
	}

	serverURLs := make([]string, len(testServers))
	for i, server := range testServers {
		serverURLs[i] = server.URL
	}

	// Инициализация компонентов
	logger := logger.NewSlogAdapter("error", false)
	repo, _ := repository.NewMemoryPool(serverURLs, logger)
	forwarder := proxy.NewHttpUtilForwarder(logger)
	lbService := app.NewLoadBalancerService(repo, forwarder, logger)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		lbService.HandleRequest(rec, req)
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	logger := logger.NewSlogAdapter("error", false)
	rateLimiter := memory.NewMemoryRateLimiter(logger)
	defer rateLimiter.Stop()

	// Устанавливаем лимит
	settings := &ratelimit.RateLimitSettings{
		ClientID:      "bench-client",
		Capacity:      1000000,
		RatePerSecond: 100000,
	}
	rateLimiter.SetRateLimit("bench-client", settings)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rateLimiter.Allow("bench-client")
	}
}

func BenchmarkRateLimiter_ConcurrentAccess(b *testing.B) {
	logger := logger.NewSlogAdapter("error", false)
	rateLimiter := memory.NewMemoryRateLimiter(logger)
	defer rateLimiter.Stop()

	// Создаем 100 клиентов
	for i := 0; i < 100; i++ {
		settings := &ratelimit.RateLimitSettings{
			ClientID:      fmt.Sprintf("client-%d", i),
			Capacity:      1000000,
			RatePerSecond: 10000,
		}
		rateLimiter.SetRateLimit(settings.ClientID, settings)
	}

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			clientID := fmt.Sprintf("client-%d", i%100)
			rateLimiter.Allow(clientID)
			i++
		}
	})
}

func BenchmarkLoadBalancer_WithRateLimit(b *testing.B) {
	// Создаем тестовый сервер
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Инициализация компонентов
	logger := logger.NewSlogAdapter("error", false)

	// Балансировщик
	repo, _ := repository.NewMemoryPool([]string{server.URL}, logger)
	forwarder := proxy.NewHttpUtilForwarder(logger)
	lbService := app.NewLoadBalancerService(repo, forwarder, logger)

	// Rate limiter
	rateLimiter := memory.NewMemoryRateLimiter(logger)
	defer rateLimiter.Stop()

	// Высокий лимит чтобы не блокировать бенчмарк
	rateLimiter.SetRateLimit("bench-client", &ratelimit.RateLimitSettings{
		ClientID:      "bench-client",
		Capacity:      1000000,
		RatePerSecond: 100000,
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-API-Key", "bench-client")
		rec := httptest.NewRecorder()

		if rateLimiter.Allow("bench-client") {
			lbService.HandleRequest(rec, req)
		}
	}
}

func BenchmarkMemoryPool_GetNextHealthyBackend(b *testing.B) {
	logger := logger.NewSlogAdapter("error", false)

	serverURLs := []string{
		"http://server1",
		"http://server2",
		"http://server3",
	}

	repo, _ := repository.NewMemoryPool(serverURLs, logger)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = repo.GetNextHealthyBackend()
	}
}
