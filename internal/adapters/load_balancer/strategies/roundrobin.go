package strategies

import (
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"sync/atomic"
)

// RoundRobinStrategy реализует стратегию Round Robin
type RoundRobinStrategy struct {
	current uint64
}

// NewRoundRobin создает новую стратегию Round Robin
func NewRoundRobin() balancer.BalancingStrategy {
	return &RoundRobinStrategy{current: 0}
}

// Name возвращает имя стратегии
func (s *RoundRobinStrategy) Name() string {
	return "RoundRobin"
}

// SelectBackend выбирает следующий бэкенд по кругу
func (s *RoundRobinStrategy) SelectBackend(backends []*balancer.Backend) (*balancer.Backend, error) {
	if len(backends) == 0 {
		return nil, balancer.ErrNoHealthyBackends // нет бэкендов для выбора
	}

	// атомарно увеличиваем счетчик и берем по модулю длины списка
	idx := atomic.AddUint64(&s.current, 1) - 1
	selected := backends[idx%uint64(len(backends))]

	return selected, nil
}
