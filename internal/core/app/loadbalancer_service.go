package app

import (
	"cloud-ru-assign/internal/core/ports"
	"errors"
	"net/http"
	"time"
)

// serviceMaxRetries максимальное количество попыток отправить запрос разным бэкендам
// можно вынести в yml конфиг
const serviceMaxRetries = 3

// loadBalancerService реализует входящий порт LoadBalancerService
// оркестрирует процесс обработки запроса
type loadBalancerService struct {
	repo      ports.BackendRepository
	forwarder ports.Forwarder
	logger    ports.Logger
}

// NewLoadBalancerService создает новый сервис балансировки
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

// HandleRequest основной метод обработки входящего запроса
func (s *loadBalancerService) HandleRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	reqLogger := s.logger.With(
		"method", r.Method,
		"uri", r.RequestURI,
		"remote_addr", r.RemoteAddr,
	)
	reqLogger.Info("начало обработки входящего запроса")

	attempts := 0
	var lastError error
	for attempts < serviceMaxRetries {
		attempts++
		attemptLogger := reqLogger.With("attempt", attempts) // логгер для конкретной попытки

		// 1 выбираем следующий здоровый бэкенд через репозиторий
		backend, found := s.repo.GetNextHealthyBackend()
		if !found {
			// если репозиторий не нашел здоровых бэкендов, нет смысла пробовать дальше
			attemptLogger.Warn("нет доступных здоровых бэкендов")
			lastError = errors.New("нет доступных здоровых бэкендов")
			break
		}

		attemptLogger = attemptLogger.With("backend_url", backend.URL.String())
		attemptLogger.Info("попытка перенаправления запроса на бэкенд")

		// 2 пересылаем запрос на выбранный бэкенд через форвардер
		err := s.forwarder.Forward(w, r, backend)

		// 3 обрабатываем результат форвардинга
		if err == nil {
			// успех
			duration := time.Since(startTime)
			attemptLogger.Info("Request forwarded successfully", "duration", duration)
			return
		}

		// ошибка при форвардинге на этот бэкенд
		lastError = err
		attemptLogger.Warn("Forwarding failed for backend", "error", err)

		// помечаем этот бэкенд как недоступный в репозитории
		s.repo.MarkBackendStatus(backend.URL, false)
		attemptLogger.Info("Marked backend as unhealthy")

		// цикл продолжится для следующей попытки с другим бэкендом
	}

	// если мы вышли из цикла, значит все попытки провалились
	duration := time.Since(startTime)
	reqLogger.Error("Failed to handle request after all retries", "attempts", attempts, "last_error", lastError, "duration", duration)

	// отвечаем клиенту ошибкой ТОЛЬКО после всех попыток, а не в момент попытки
	http.Error(w, "Service Unavailable (Failed after multiple attempts)", http.StatusServiceUnavailable)
}
