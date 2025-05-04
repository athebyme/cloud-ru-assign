package http

import (
	"context"
	"errors"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"net/http"
	"time"
)

// ServerAdapter управляет жизненным циклом HTTP сервера и направляет запросы в сервис ядра
type ServerAdapter struct {
	httpServer *http.Server
	lbService  ports.LoadBalancerService
	logger     ports.Logger
	done       chan struct{}
	Server     *http.Server
}

// NewServerAdapter создает новый адаптер HTTP сервера
func NewServerAdapter(
	listenAddr string,
	lbService ports.LoadBalancerService,
	logger ports.Logger,
) *ServerAdapter {
	adapterLogger := logger.With("adapter", "HTTPServer")
	mux := http.NewServeMux() // мультиплексор запросов

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
		Server:     srv,
	}
}

// Run запускает прослушивание сервером входящих запросов в отдельной горутине
// не блокирует выполнение
func (s *ServerAdapter) Run() {
	s.logger.Info("HTTP server adapter starting", "address", s.httpServer.Addr)
	go func() {
		defer close(s.done) // сигнализируем о завершении при выходе

		// ListenAndServe всегда возвращает ошибку, проверяем что это не ErrServerClosed
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("ошибка запуска адаптера HTTP сервера", "error", err)
		} else {
			s.logger.Info("адаптер HTTP сервера прекратил прослушивание")
		}
	}()
}

// Stop корректно останавливает HTTP сервер
func (s *ServerAdapter) Stop(ctx context.Context) {
	s.logger.Info("инициация корректной остановки адаптера HTTP сервера")

	// пытаемся остановить сервер с ожиданием завершения текущих запросов
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("ошибка при корректной остановке адаптера HTTP сервера", "error", err)

		// если Shutdown не удался, пробуем закрыть принудительно
		if closeErr := s.httpServer.Close(); closeErr != nil {
			s.logger.Error("ошибка при принудительном закрытии адаптера HTTP сервера", "error", closeErr)
		}
	} else {
		s.logger.Info("адаптер HTTP сервера корректно остановлен")
	}
}
