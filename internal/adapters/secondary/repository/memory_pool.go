package repository

import (
	"fmt"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"math/rand"
	"net/url"
	"sync"
	"sync/atomic"
)

const (
	StrategyRoundRobin       = "round-robin"
	StrategyLeastConnections = "least-connections"
	StrategyRandom           = "random"
)

type BackendState struct {
	balancer.Backend
	alive atomic.Bool
}

func (bs *BackendState) SetAlive(alive bool) { bs.alive.Store(alive) }
func (bs *BackendState) IsAlive() bool       { return bs.alive.Load() }

// MemoryPool реализует ports.BackendRepository
// использует переданную стратегию для выбора бэкенда
type MemoryPool struct {
	backends       []*BackendState
	current        uint64
	mux            sync.RWMutex
	logger         ports.Logger
	strategy       string
	connections    map[string]int
	connectionsMux sync.RWMutex // либо синк мапу
}

// NewMemoryPool создает новый in-memory репозиторий
func NewMemoryPool(backendUrls []string, logger ports.Logger) (*MemoryPool, error) {
	poolLogger := logger.With("adapter", "MemoryPool")
	var backends []*BackendState

	if len(backendUrls) == 0 {
		return nil, fmt.Errorf("список URL бэкендов пуст")
	}

	for _, rawUrl := range backendUrls {
		parsedUrl, err := url.Parse(rawUrl)
		if err != nil {
			poolLogger.Warn("пропускаем невалидный URL бэкенда", "url", rawUrl, "error", err)
			continue
		}
		state := &BackendState{
			Backend: balancer.Backend{URL: parsedUrl},
		}

		state.SetAlive(true) // изначально считаем доступным

		backends = append(backends, state)
		poolLogger.Debug("добавлен бэкенд в пул", "url", rawUrl)
	}

	if len(backends) == 0 {
		return nil, fmt.Errorf("не найдено валидных бэкендов в предоставленном списке")
	}

	poolLogger.Info("in-memory пул инициализирован", "backend_count", len(backends))
	return &MemoryPool{
		backends:    backends,
		current:     0,
		logger:      poolLogger,
		strategy:    StrategyRoundRobin, // по умолчанию
		connections: make(map[string]int),
	}, nil
}

func (p *MemoryPool) SetStrategy(strategy string) error {
	switch strategy {
	case StrategyRoundRobin, StrategyLeastConnections, StrategyRandom:
		p.mux.Lock()
		defer p.mux.Unlock()
		p.strategy = strategy
		p.logger.Info("стратегия балансировки изменена", "strategy", strategy)
		return nil
	default:
		return fmt.Errorf("неподдерживаемая стратегия: %s", strategy)
	}
}

// GetBackends реализует ports.BackendRepository
func (p *MemoryPool) GetBackends() []*balancer.Backend {
	p.mux.RLock()
	defer p.mux.RUnlock()
	domainBackends := make([]*balancer.Backend, len(p.backends))
	for i, state := range p.backends {
		domainBackends[i] = &state.Backend
	}
	return domainBackends
}

// MarkBackendStatus реализует ports.BackendRepository
// атомарно обновляет статус бэкенда
func (p *MemoryPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	p.mux.RLock()
	defer p.mux.RUnlock()
	urlStr := backendUrl.String()
	found := false
	for _, b := range p.backends {
		if b.URL.String() == urlStr {
			currentState := b.IsAlive()

			// логируем только если статус действительно изменился
			if currentState != alive {
				b.SetAlive(alive)
				p.logger.Info("статус бэкенда обновлен", "url", backendUrl, "new_status", alive)
			}
			found = true
		}
	}
	if !found {
		p.logger.Warn("попытка обновить статус неизвестного бэкенда", "url", backendUrl)
	}
}

func (p *MemoryPool) GetNextHealthyBackend() (*balancer.Backend, bool) {
	p.mux.RLock()
	defer p.mux.RUnlock()

	if len(p.backends) == 0 {
		p.logger.Warn("GetNextHealthyBackend called on empty pool")
		return nil, false
	}

	var selected *balancer.Backend
	var found bool

	switch p.strategy {
	case StrategyRoundRobin:
		selected, found = p.getRoundRobinBackend()
	case StrategyLeastConnections:
		selected, found = p.getLeastConnectionsBackend()
	case StrategyRandom:
		selected, found = p.getRandomBackend()
	default:
		p.logger.Warn("неизвестная стратегия, используется round-robin", "strategy", p.strategy)
		selected, found = p.getRoundRobinBackend()
	}

	return selected, found
}

func (p *MemoryPool) getLeastConnectionsBackend() (*balancer.Backend, bool) {
	var selectedBackend *balancer.Backend
	minConnections := int(^uint(0) >> 1) // максимальное int значение

	for _, backendState := range p.backends {
		if backendState.IsAlive() {
			connections := p.getConnectionCount(backendState.URL.String())

			// бэкенд с меньшим количеством соединений
			if connections < minConnections {
				minConnections = connections
				selectedBackend = &backendState.Backend
			}
		}
	}

	if selectedBackend != nil {
		p.logger.Debug("выбран бэкенд с минимумом соединений",
			"url", selectedBackend.URL.String(),
			"connections", minConnections)
		return selectedBackend, true
	}

	p.logger.Warn("No healthy backend found in pool (least-connections)")
	return nil, false
}

func (p *MemoryPool) getRandomBackend() (*balancer.Backend, bool) {
	aliveBackends := make([]*BackendState, 0)

	for _, backendState := range p.backends {
		if backendState.IsAlive() {
			aliveBackends = append(aliveBackends, backendState)
		}
	}

	if len(aliveBackends) == 0 {
		p.logger.Warn("No healthy backend found in pool (random)")
		return nil, false
	}

	randomIndex := rand.Intn(len(aliveBackends))
	selectedBackend := &aliveBackends[randomIndex].Backend

	p.logger.Debug("выбран случайный бэкенд", "url", selectedBackend.URL.String())
	return selectedBackend, true
}

func (p *MemoryPool) GetActiveConnections(backend *balancer.Backend) int {
	p.connectionsMux.RLock()
	defer p.connectionsMux.RUnlock()
	return p.connections[backend.URL.String()]
}

func (p *MemoryPool) IncrementConnections(backend *balancer.Backend) {
	p.connectionsMux.Lock()
	defer p.connectionsMux.Unlock()
	p.connections[backend.URL.String()]++
	p.logger.Debug("соединения увеличены", "url", backend.URL.String(), "connections", p.connections[backend.URL.String()])
}

func (p *MemoryPool) DecrementConnections(backend *balancer.Backend) {
	p.connectionsMux.Lock()
	defer p.connectionsMux.Unlock()

	if p.connections[backend.URL.String()] > 0 {
		p.connections[backend.URL.String()]--
		p.logger.Debug("соединения уменьшены", "url", backend.URL.String(), "connections", p.connections[backend.URL.String()])
	}
}

func (p *MemoryPool) getConnectionCount(url string) int {
	p.connectionsMux.RLock()
	defer p.connectionsMux.RUnlock()
	return p.connections[url]
}

func (p *MemoryPool) getRoundRobinBackend() (*balancer.Backend, bool) {
	numBackends := uint64(len(p.backends))
	if numBackends == 0 {
		p.logger.Warn("getRoundRobinBackend called on empty pool")
		return nil, false
	}

	nextIndex := atomic.AddUint64(&p.current, 1) - 1

	for i := uint64(0); i < numBackends; i++ {
		idx := (nextIndex + i) % numBackends
		currentBackendState := p.backends[idx]

		if currentBackendState.IsAlive() {
			p.logger.Debug("выбран здоровый бэкенд (round-robin)", "url", currentBackendState.URL.String())
			return &currentBackendState.Backend, true
		}
	}

	p.logger.Warn("No healthy backend found in pool (round-robin)")
	return nil, false
}

var _ ports.BackendRepository = (*MemoryPool)(nil) // compile чек на то, что все мем пул имплементит интерфейс репо
