package proxy

import (
	"cloud-ru-assign/internal/core/domain"
	"cloud-ru-assign/internal/core/ports"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"
)

// HttpUtilForwarder реализует порт ports.Forwarder, используя net/http/httputil
type HttpUtilForwarder struct {
	logger ports.Logger
}

// NewHttpUtilForwarder создает новый адаптер форвардера
func NewHttpUtilForwarder(logger ports.Logger) ports.Forwarder {
	return &HttpUtilForwarder{
		logger: logger.With("adapter", "HttputilForwarder"),
	}
}

// Forward реализует ports.Forwarder
// проксирует запрос r на бэкенд target, используя w для ответа
func (f *HttpUtilForwarder) Forward(w http.ResponseWriter, r *http.Request, target *domain.Backend) error {
	// создаем реверс-прокси для конкретного бэкенда
	proxy := httputil.NewSingleHostReverseProxy(target.URL)
	proxyLogger := f.logger.With("target_url", target.URL.String())

	// переменная для захвата ошибки из ErrorHandler (требует синхронизации)
	var proxyErr error
	var mu sync.Mutex // мьютекс для защиты доступа к proxyErr

	// кастомный обработчик ошибок прокси
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		mu.Lock() //  захватываем мьютекс для безопасной записи

		// сохраняем ошибку, чтобы вернуть ее из Forward
		proxyErr = fmt.Errorf("ошибка проксирования на %s: %w", target.URL, err)
		mu.Unlock()

		// стандартный ErrorHandler пытается записать 502 Bad Gateway,
		// мы не можем это предотвратить надежно, но ошибку захватили
		proxyLogger.Warn("сработал ErrorHandler реверс-прокси", "error", err)

		// не пишем здесь ответ, позволяем вызывающей стороне (сервису) решить, что делать дальше
		// (например, пометить бэкенд как мертвый и попробовать другой)
	}

	// модифицируем запрос перед отправкой на бэкенд
	originalDirector := proxy.Director // сохраняем стандартный директор
	proxy.Director = func(req *http.Request) {
		originalDirector(req)      // выполняем стандартные действия (копирование и тд)
		req.Host = target.URL.Host // устанавливаем правильный Host для бэкенда (важно для vhost)

		// заголовки
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}
		req.Header.Set("X-Forwarded-Proto", proto)
		if originalHost := r.Header.Get("X-Forwarded-Host"); originalHost != "" {
			req.Header.Set("X-Forwarded-Host", originalHost)
		} else {
			req.Header.Set("X-Forwarded-Host", r.Host) // используем оригинальный Host, если X-Forwarded-Host не было
		}
		proxyLogger.Debug("модифицирован запрос в директоре", "host", req.Host, "x-fwd-for", req.Header.Get("X-Forwarded-For"))
	}

	proxyLogger.Debug("вызов ServeHTTP для перенаправления запроса")

	// выполняем проксирование (блокирующая операция)
	proxy.ServeHTTP(w, r)

	// проверяем, была ли установлена ошибка в ErrorHandler
	mu.Lock()
	capturedErr := proxyErr
	mu.Unlock()

	if capturedErr != nil {
		proxyLogger.Debug("перенаправление завершилось с ошибкой, захваченной ErrorHandler", "error", capturedErr)
		return capturedErr // возвращаем ошибку, если она была
	}

	// если ErrorHandler не сработал, считаем, что проксирование прошло успешно
	// (бэкенд мог вернуть свою ошибку, но само проксирование удалось)
	proxyLogger.Debug("перенаправление завершено без срабатывания ErrorHandler")
	return nil // нет ошибки проксирования
}
