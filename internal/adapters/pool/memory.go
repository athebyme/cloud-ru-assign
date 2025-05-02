package pool

import (
	"cloud-ru-assign/internal/core/domain"
	"cloud-ru-assign/internal/core/ports"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
)

type BackendState struct {
	domain.Backend
	alive atomic.Bool
}

func (bs *BackendState) SetAlive(alive bool) { bs.alive.Store(alive) }
func (bs *BackendState) IsAlive() bool       { return bs.alive.Load() }

type MemoryPool struct {
	backends []*BackendState
	current  uint64
	mux      sync.RWMutex
	logger   ports.Logger
}

func NewMemoryPool(backendUrls []string, logger ports.Logger) (*MemoryPool, error) {
	poolLogger := logger.With("adapter", "MemoryPool")
	var backends []*BackendState

	if len(backendUrls) == 0 {
		return nil, fmt.Errorf("backend URL list is empty")
	}
	for _, rawUrl := range backendUrls {
		parsedUrl, err := url.Parse(rawUrl)
		if err != nil {
			poolLogger.Warn("Skipping invalid backend URL", "url", rawUrl, "error", err)
			continue
		}
		state := &BackendState{
			Backend: domain.Backend{URL: parsedUrl},
		}
		state.SetAlive(true)
		backends = append(backends, state)
		poolLogger.Debug("Added backend to pool", "url", rawUrl)
	}

	if len(backends) == 0 {
		return nil, fmt.Errorf("no valid backends found in the provided list")
	}

	poolLogger.Info("Memory pool initialized", "backend_count", len(backends))
	return &MemoryPool{
		backends: backends,
		current:  0,
		logger:   poolLogger,
	}, nil
}

func (p *MemoryPool) GetBackends() []*domain.Backend {
	p.mux.RLock()
	defer p.mux.RUnlock()
	domainBackends := make([]*domain.Backend, len(p.backends))
	for i, state := range p.backends {
		domainBackends[i] = &state.Backend
	}
	return domainBackends
}

func (p *MemoryPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	p.mux.RLock()
	defer p.mux.RUnlock()
	urlStr := backendUrl.String()
	found := false
	for _, b := range p.backends {
		if b.URL.String() == urlStr {
			currentState := b.IsAlive()
			if currentState != alive {
				b.SetAlive(alive)
				p.logger.Info("Backend status updated", "url", backendUrl, "new_status", alive)
			}
			found = true
		}
	}
	if !found {
		p.logger.Warn("Attempt to mark status for unknown backend", "url", backendUrl)
	}
}
func (p *MemoryPool) GetNextHealthyBackend() (*domain.Backend, bool) {
	p.mux.RLock()
	numBackends := uint64(len(p.backends))
	if numBackends == 0 {
		p.mux.RUnlock()
		p.logger.Warn("GetNextHealthyBackend called on empty pool")
		return nil, false
	}

	nextIndex := atomic.AddUint64(&p.current, 1) - 1

	for i := uint64(0); i < numBackends; i++ {
		idx := (nextIndex + i) % numBackends
		currentBackendState := p.backends[idx]

		if currentBackendState.IsAlive() {
			p.mux.RUnlock()
			p.logger.Debug("Selected healthy backend", "url", currentBackendState.URL)
			return &currentBackendState.Backend, true
		}
	}

	p.mux.RUnlock()
	p.logger.Warn("No healthy backend found in pool")
	return nil, false
}

var _ ports.BackendRepository = (*MemoryPool)(nil)
