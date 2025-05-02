package proxy

import (
	"cloud-ru-assign/internal/core/domain"
	"cloud-ru-assign/internal/core/ports"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"
)

type HttpUtilForwarder struct {
	logger ports.Logger
}

func NewHttpUtilForwarder(logger ports.Logger) ports.Forwarder {
	return &HttpUtilForwarder{
		logger: logger.With("adapter", "HttputilForwarder"),
	}
}

func (f *HttpUtilForwarder) Forward(w http.ResponseWriter, r *http.Request, target *domain.Backend) error {
	proxy := httputil.NewSingleHostReverseProxy(target.URL)
	proxyLogger := f.logger.With("target_url", target.URL.String())

	var proxyErr error
	var mu sync.Mutex

	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		mu.Lock()
		proxyErr = fmt.Errorf("proxy error to %s: %w", target.URL, err)
		mu.Unlock()
		proxyLogger.Warn("Reverse proxy ErrorHandler triggered", "error", err)
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.URL.Host
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}
		req.Header.Set("X-Forwarded-Proto", proto)
		if originalHost := r.Header.Get("X-Forwarded-Host"); originalHost != "" {
			req.Header.Set("X-Forwarded-Host", originalHost)
		} else {
			req.Header.Set("X-Forwarded-Host", r.Host)
		}
		proxyLogger.Debug("Request director modified headers", "host", req.Host, "x-fwd-for", req.Header.Get("X-Forwarded-For"))
	}

	proxyLogger.Debug("Serving request via reverse proxy")
	proxy.ServeHTTP(w, r)

	mu.Lock()
	capturedErr := proxyErr
	mu.Unlock()

	if capturedErr != nil {
		proxyLogger.Debug("Forwarding resulted in error captured by ErrorHandler", "error", capturedErr)
		return capturedErr
	}

	proxyLogger.Debug("Forwarding completed without triggering ErrorHandler")
	return nil
}
