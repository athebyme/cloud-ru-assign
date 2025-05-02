package http

import (
	"cloud-ru-assign/internal/core/ports"
	"context"
	"errors"
	"net/http"
	"time"
)

type ServerAdapter struct {
	httpServer *http.Server
	lbService  ports.LoadBalancerService
	logger     ports.Logger
	done       chan struct{}
}

func NewServerAdapter(
	listenAddr string,
	lbService ports.LoadBalancerService,
	logger ports.Logger,
) *ServerAdapter {
	adapterLogger := logger.With("adapter", "HTTPServer")
	mux := http.NewServeMux()

	mux.HandleFunc("/", lbService.HandleRequest)

	errorLog := ports.NewSlogLogger(adapterLogger.With("source", "http_server_internal"))
	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  30 * time.Second,
		ErrorLog:     errorLog,
	}

	return &ServerAdapter{
		httpServer: srv,
		lbService:  lbService,
		logger:     adapterLogger,
		done:       make(chan struct{}),
	}
}

func (s *ServerAdapter) Run() {
	s.logger.Info("HTTP server adapter starting", "address", s.httpServer.Addr)
	go func() {
		defer close(s.done)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server adapter ListenAndServe error", "error", err)
		} else {
			s.logger.Info("HTTP server adapter stopped listening.")
		}
	}()
}

func (s *ServerAdapter) Stop(ctx context.Context) {
	s.logger.Info("HTTP server adapter initiating graceful shutdown...")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("HTTP server adapter graceful shutdown failed", "error", err)
		if closeErr := s.httpServer.Close(); closeErr != nil {
			s.logger.Error("HTTP server adapter forceful close failed", "error", closeErr)
		}
	} else {
		s.logger.Info("HTTP server adapter graceful shutdown completed.")
	}
}
