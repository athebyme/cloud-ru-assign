package load_balancer

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
)

type Backend struct {
	URL   *url.URL
	alive atomic.Bool
}

func NewBackend(rawUrl string) (*Backend, error) {
	parsedUrl, err := url.Parse(rawUrl)
	if err != nil {
		return nil, fmt.Errorf("невалидный URL бэкенда '%s': %w", rawUrl, err)
	}
	backend := &Backend{
		URL: parsedUrl,
	}
	backend.SetAlive(true)
	return backend, nil
}

func (b *Backend) SetAlive(alive bool) {
	b.alive.Store(alive)
}

func (b *Backend) IsAlive() bool {
	return b.alive.Load()
}

type ServerPool struct {
	backends []*Backend
	current  uint64
	mux      sync.RWMutex
}

func NewServerPool(backendUrls []string) (*ServerPool, error) {
	var backends []*Backend
	if len(backendUrls) == 0 {
		return nil, fmt.Errorf("список URL бэкендов пуст")
	}

	for _, rawUrl := range backendUrls {
		backend, err := NewBackend(rawUrl)
		if err != nil {
			log.Printf("Ошибка создания бэкенда для URL '%s': %v. Пропускаем.", rawUrl, err)
			continue
		}
		backends = append(backends, backend)
	}

	if len(backends) == 0 {
		return nil, fmt.Errorf("не удалось создать ни одного валидного бэкенда из предоставленного списка")
	}

	log.Printf("Создан пул с %d бэкендами", len(backends))
	return &ServerPool{
		backends: backends,
		current:  0,
	}, nil
}

func (s *ServerPool) GetNextHealthyPeer() *Backend {
	s.mux.RLock()
	numBackends := uint64(len(s.backends))
	if numBackends == 0 {
		s.mux.RUnlock()
		return nil
	}

	startIndex := atomic.AddUint64(&s.current, 1) - 1

	for i := uint64(0); i < numBackends; i++ {
		idx := (startIndex + i) % numBackends
		currentBackend := s.backends[idx]
		s.mux.RUnlock()

		if currentBackend.IsAlive() {
			// не идеально точно при высокой конкуренции, но ок для round-robin
			atomic.StoreUint64(&s.current, idx+1)
			log.Printf("Выбран здоровый бэкенд: %s", currentBackend.URL)
			return currentBackend
		}

		log.Printf("Бэкенд %s не доступен, ищем следующий", currentBackend.URL)
		s.mux.RLock()
	}

	s.mux.RUnlock()
	log.Println("Предупреждение: не найдено ни одного доступного бэкенда в пуле!")
	return nil
}

func (s *ServerPool) GetBackends() []*Backend {
	s.mux.RLock()
	defer s.mux.RUnlock()
	backendsCopy := make([]*Backend, len(s.backends))
	copy(backendsCopy, s.backends)
	return backendsCopy
}

func (s *ServerPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	s.mux.RLock()
	defer s.mux.RUnlock()

	for _, b := range s.backends {
		if b.URL.String() == backendUrl.String() {
			b.SetAlive(alive)
			log.Printf("Статус бэкенда %s изменен на %t", backendUrl, alive)
			return
		}
	}
	log.Printf("Попытка изменить статус несуществующего бэкенда: %s", backendUrl)
}
