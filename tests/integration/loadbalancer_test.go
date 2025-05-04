package integration

import (
	"fmt"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/logger"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/proxy"
	"github.com/athebyme/cloud-ru-assign/internal/adapters/secondary/repository"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/athebyme/cloud-ru-assign/internal/core/app"
)

func TestLoadBalancer_RoundRobin(t *testing.T) {
	responses := []string{"backend1", "backend2", "backend3"}
	var servers []*httptest.Server
	var serverURLs []string

	for _, response := range responses {
		resp := response
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(resp))
		}))
		servers = append(servers, server)
		serverURLs = append(serverURLs, server.URL)
	}
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	logger := logger.NewSlogAdapter("info", false)
	repo, err := repository.NewMemoryPool(serverURLs, logger)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	forwarder := proxy.NewHttpUtilForwarder(logger)
	lbService := app.NewLoadBalancerService(repo, forwarder, logger)

	var mu sync.Mutex
	counts := make(map[string]int)

	for i := 0; i < 9; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		lbService.HandleRequest(rec, req)

		body := rec.Body.String()
		mu.Lock()
		counts[body]++
		mu.Unlock()
	}

	for backend, count := range counts {
		if count != 3 {
			t.Errorf("Backend %s received %d requests, expected 3", backend, count)
		}
	}
}

func TestLoadBalancer_HealthCheck(t *testing.T) {
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.Write([]byte("healthy response"))
		}
	}))
	defer healthyServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer unhealthyServer.Close()

	serverURLs := []string{healthyServer.URL, unhealthyServer.URL}

	logger := logger.NewSlogAdapter("info", false)
	repo, _ := repository.NewMemoryPool(serverURLs, logger)
	forwarder := proxy.NewHttpUtilForwarder(logger)
	lbService := app.NewLoadBalancerService(repo, forwarder, logger)

	unhealthyURL, _ := url.Parse(unhealthyServer.URL)
	repo.MarkBackendStatus(unhealthyURL, false)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()
		lbService.HandleRequest(rec, req)

		if rec.Body.String() != "healthy response" {
			t.Errorf("Expected response from healthy backend, got: %s", rec.Body.String())
		}
	}
}

func TestLoadBalancer_ConcurrentRequests(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	logger := logger.NewSlogAdapter("error", false)
	repo, _ := repository.NewMemoryPool([]string{backend.URL}, logger)
	forwarder := proxy.NewHttpUtilForwarder(logger)
	lbService := app.NewLoadBalancerService(repo, forwarder, logger)

	const concurrency = 100
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			lbService.HandleRequest(rec, req)

			if rec.Code != http.StatusOK {
				errors <- fmt.Errorf("unexpected status code: %d", rec.Code)
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}
