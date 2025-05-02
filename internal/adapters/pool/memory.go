package pool

import (
	"cloud-ru-assign/internal/core/domain"
	"cloud-ru-assign/internal/core/ports"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
)

// BackendState хранит доменную сущность Backend и ее текущий статус alive
// используется только внутри этого адаптера
type BackendState struct {
	domain.Backend
	alive atomic.Bool // флаг доступности для конкурентной среды
}

// SetAlive атомарно устанавливает статус доступности
func (bs *BackendState) SetAlive(alive bool) { bs.alive.Store(alive) }

// IsAlive атомарно читает статус доступности
func (bs *BackendState) IsAlive() bool { return bs.alive.Load() }

// MemoryPool реализует порт ports.BackendRepository, используя in-memory слайс
// реализует стратегию выбора Round Robin
// TODO : паттерн стратегии или команды
type MemoryPool struct {
	backends []*BackendState // слайс состояний бэкендов
	current  uint64          // атомарный счетчик для round robin
	mux      sync.RWMutex    // мьютекс для защиты доступа к слайсу backends (на случай динамического изменения)
	logger   ports.Logger
}

// NewMemoryPool создает новый in-memory репозиторий бэкендов
func NewMemoryPool(backendUrls []string, logger ports.Logger) (*MemoryPool, error) {
	poolLogger := logger.With("adapter", "MemoryPool")
	var backends []*BackendState

	if len(backendUrls) == 0 {
		return nil, fmt.Errorf("список URL бэкендов пуст")
	}

	// создаем состояния для каждого валидного URL
	for _, rawUrl := range backendUrls {
		parsedUrl, err := url.Parse(rawUrl)
		if err != nil {
			poolLogger.Warn("пропускаем невалидный URL бэкенда", "url", rawUrl, "error", err)
			continue
		}
		state := &BackendState{
			Backend: domain.Backend{URL: parsedUrl},
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
		backends: backends,
		current:  0,
		logger:   poolLogger,
	}, nil
}

// GetBackends реализует ports.BackendRepository
// возвращает список доменных сущностей бэкендов
func (p *MemoryPool) GetBackends() []*domain.Backend {
	p.mux.RLock() // блокировка на чтение дабы избежать race condition
	defer p.mux.RUnlock()
	domainBackends := make([]*domain.Backend, len(p.backends))
	for i, state := range p.backends {
		domainBackends[i] = &state.Backend // возвращаем указатель на встроенную доменную сущность
	}
	return domainBackends
}

// MarkBackendStatus реализует ports.BackendRepository
// атомарно обновляет статус бэкенда
func (p *MemoryPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	p.mux.RLock() // достаточно RLock, так как меняем только атомарный флаг
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

// GetNextHealthyBackend реализует ports.BackendRepository
// выбирает следующий живой бэкенд по алгоритму Round Robin
func (p *MemoryPool) GetNextHealthyBackend() (*domain.Backend, bool) {
	p.mux.RLock()
	numBackends := uint64(len(p.backends))
	if numBackends == 0 {
		p.mux.RUnlock()
		p.logger.Warn("GetNextHealthyBackend called on empty pool")
		return nil, false
	}

	nextIndex := atomic.AddUint64(&p.current, 1) - 1

	// проходим по всем бэкендам (максимум один полный круг)
	for i := uint64(0); i < numBackends; i++ {
		// вычисляем индекс текущего кандидата с учетом смещения и длины слайса
		idx := (nextIndex + i) % numBackends
		currentBackendState := p.backends[idx] // состояние бэкенда

		// проверяем, жив ли он (атомарно)
		if currentBackendState.IsAlive() {
			p.mux.RUnlock() // разблокируем мьютекс перед возвратом дабы не вылететь в дедлок
			p.logger.Debug("выбран здоровый бэкенд", "url", currentBackendState.URL)
			return &currentBackendState.Backend, true // найдено
		}
	}

	// если прошли весь круг и не нашли живых
	p.mux.RUnlock()
	p.logger.Warn("No healthy backend found in pool")
	return nil, false // не найдено
}

var _ ports.BackendRepository = (*MemoryPool)(nil) // compile чек на то, что все мем пул имплементит интерфейс репо
