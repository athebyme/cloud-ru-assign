package middleware

import (
	"encoding/json"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"net"
	"net/http"
	"time"
)

// RateLimitMiddleware создает HTTP middleware для rate limiting
func RateLimitMiddleware(rateLimiter ports.RateLimiter, logger ports.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientID := getClientID(r)

			if !rateLimiter.Allow(clientID) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)

				errorResponse := map[string]interface{}{
					"code":    429,
					"message": "Rate limit exceeded",
					"time":    time.Now().UTC(),
				}

				if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
					logger.Error("Failed to encode rate limit error", "error", err)
				}

				logger.Info("Rate limit exceeded", "client", clientID, "uri", r.RequestURI)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func getClientID(r *http.Request) string {
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return "api_" + apiKey
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return "ip_" + ip
}
