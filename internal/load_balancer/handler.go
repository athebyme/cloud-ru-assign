package load_balancer

import (
	"log"
	"net/http"
	"net/http/httputil"
	"time"
)

const maxRetries = 3

func NewLoadBalancerHandler(pool *ServerPool) http.Handler {
	if pool == nil {
		log.Fatal("Невозможно создать обработчик: пул серверов не инициализирован (nil)")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()
		attempts := 0

		for attempts < maxRetries {
			attempts++
			peer := pool.GetNextHealthyPeer()
			if peer == nil {
				log.Printf("[Попытка %d] Нет доступных бэкендов для запроса %s %s", attempts, r.Method, r.RequestURI)
				http.Error(w, "Service not available", http.StatusServiceUnavailable)
				return
			}

			log.Printf("[Попытка %d] Перенаправление [%s] %s -> %s", attempts, r.Method, r.RequestURI, peer.URL)

			proxy := httputil.NewSingleHostReverseProxy(peer.URL)

			var proxyError error

			proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
				log.Printf("Ошибка проксирования на бэкенд %s [Попытка %d]: %v", peer.URL, attempts, err)
				pool.MarkBackendStatus(peer.URL, false)
				proxyError = err
			}

			originalDirector := proxy.Director
			proxy.Director = func(req *http.Request) {
				originalDirector(req)
				req.Host = peer.URL.Host
				req.Header.Set("X-Forwarded-For", r.RemoteAddr)
				req.Header.Set("X-Forwarded-Proto", r.URL.Scheme)
				if originalHost := r.Header.Get("X-Forwarded-Host"); originalHost != "" {
					req.Header.Set("X-Forwarded-Host", originalHost)
				} else {
					req.Header.Set("X-Forwarded-Host", r.Host)
				}
				req.Header.Set("X-Real-IP", r.RemoteAddr)
			}

			proxy.ServeHTTP(w, r)

			if proxyError == nil {
				duration := time.Since(startTime)
				log.Printf("Запрос [%s] %s успешно обработан за %v (бэкенд: %s, попытка: %d)", r.Method, r.RequestURI, duration, peer.URL, attempts)
				return
			}

			log.Printf("Попытка %d не удалась. Пробуем следующий бэкенд.", attempts)

			time.Sleep(50 * time.Millisecond)
		}

		log.Printf("Все %d попыток обработать запрос %s %s не удались.", maxRetries, r.Method, r.RequestURI)
		http.Error(w, "Service Unavailable (all backends failed)", http.StatusServiceUnavailable)
		duration := time.Since(startTime)
		log.Printf("Запрос [%s] %s НЕ обработан после %d попыток за %v", r.Method, r.RequestURI, attempts, duration)

	})
}
