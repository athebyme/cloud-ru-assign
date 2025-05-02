package ports

import "net/http"

// LoadBalancerService определяет основной входящий порт для обработки запросов
// это интерфейс, который ядро предоставляет внешнему миру (например, HTTP адаптеру)
type LoadBalancerService interface {
	HandleRequest(w http.ResponseWriter, r *http.Request)
}
