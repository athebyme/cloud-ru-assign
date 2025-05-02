package app

import (
	"cloud-ru-assign/internal/core/ports"
	"net/http"
	"time"
)

const serviceMaxRetries = 3

type loadBalancerService struct {
	repo      ports.BackendRepository
	forwarder ports.Forwarder
	logger    ports.Logger
}

func NewLoadBalancerService(
	repo ports.BackendRepository,
	forwarder ports.Forwarder,
	logger ports.Logger,
) ports.LoadBalancerService {
	return &loadBalancerService{
		repo:      repo,
		forwarder: forwarder,
		logger:    logger.With("service", "LoadBalancerService"),
	}
}

func (s *loadBalancerService) HandleRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	reqLogger := s.logger.With(
		"method", r.Method,
		"uri", r.RequestURI,
		"remote_addr", r.RemoteAddr,
	)
	reqLogger.Info("Handling incoming request")

	attempts := 0
	var lastError error
	for attempts < serviceMaxRetries {
		attempts++
		attemptLogger := reqLogger.With("attempt", attempts)

		backend, found := s.repo.GetNextHealthyBackend()
		if !found {
			reqLogger.Error("Failed to find any healthy backend via repository")
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		attemptLogger = attemptLogger.With("backend_url", backend.URL.String())
		attemptLogger.Info("Attempting to forward request")

		err := s.forwarder.Forward(w, r, backend)

		if err == nil {
			// ok
			duration := time.Since(startTime)
			attemptLogger.Info("Request forwarded successfully", "duration", duration)
			return
		}

		lastError = err
		attemptLogger.Warn("Forwarding failed for backend", "error", err)

		s.repo.MarkBackendStatus(backend.URL, false)
		attemptLogger.Info("Marked backend as unhealthy")
	}

	duration := time.Since(startTime)
	reqLogger.Error("Failed to handle request after all retries", "attempts", attempts, "last_error", lastError, "duration", duration)
	http.Error(w, "Service Unavailable (Failed after multiple attempts)", http.StatusServiceUnavailable)
}
