package balancer

import "errors"

var ErrNoHealthyBackends = errors.New("нет доступных здоровых бэкендов для выбора")

// BalancingStrategy определяет интерфейс для алгоритмов выбора бэкенда
// каждая реализация представляет собой отдельный алгоритм балансировки
type BalancingStrategy interface {
	// SelectBackend выбирает один бэкенд из списка доступных (здоровых)
	// возвращает выбранный бэкенд или ошибку (например, ErrNoHealthyBackends)
	SelectBackend(backends []*Backend) (*Backend, error)
	// Name возвращает имя стратегии (для логирования/конфигурации)
	Name() string
}
