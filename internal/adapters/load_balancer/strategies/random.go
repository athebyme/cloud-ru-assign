package strategies

import (
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"math/rand"
	"time"
)

// RandomStrategy реализует стратегию случайного выбора
type RandomStrategy struct {
	// можно добавить свой источник rand для потокобезопасности и лучшей производительности,
	// но для простоты пока используем глобальный rand
	rnd *rand.Rand
}

// NewRandom создает новую стратегию случайного выбора
func NewRandom() balancer.BalancingStrategy {
	// инициализируем глобальный rand один раз (или использовать sync.Once)
	// В Go 1.20+ math/rand потокобезопасен и сидируется автоматически,
	// поэтому явный Seed не обязателен, если версия Go позволяет.
	// rand.Seed(time.Now().UnixNano()) // для версий до 1.20

	return &RandomStrategy{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())), // вариант с локальным генератором
	}
}

// Name возвращает имя стратегии
func (s *RandomStrategy) Name() string {
	return "Random"
}

// SelectBackend выбирает случайный бэкенд из списка
func (s *RandomStrategy) SelectBackend(backends []*balancer.Backend) (*balancer.Backend, error) {
	if len(backends) == 0 {
		return nil, balancer.ErrNoHealthyBackends
	}

	idx := rand.Intn(len(backends))
	selected := backends[idx]

	return selected, nil
}
