package healthcheck

import (
	"fmt"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"net/http"
	"net/url"
	"time"
)

// HTTPChecker реализует порт HealthChecker, используя HTTP GET запросы
type HTTPChecker struct {
	client  *http.Client
	timeout time.Duration
	path    string
}

// NewHTTPChecker создает новый HTTP health checker
func NewHTTPChecker(timeout time.Duration, path string) ports.HealthChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				// отключаем keep-alive, тк проверки в данном варианте короткоживущие и редкие
				DisableKeepAlives: true,
				// можно было бы сделать таймауты для соединений, но в данном варианте упрощено все
			},
		},
		timeout: timeout,
		path:    path,
	}
}

// Check выполняет HTTP GET запрос к целевому URL
func (c *HTTPChecker) Check(target *url.URL) error {
	checkURL := target.String()
	if c.path != "" { // если есть конкретный специфичный путь из конфига, он будет != ""
		checkURL = target.JoinPath(c.path).String()
	}
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		// ошибка создания запроса (маловероятно для GET)
		return fmt.Errorf("не удалось создать запрос health check для %s: %w", target, err)
	}

	req.Header.Set("User-Agent", "LoadBalancer-HealthChecker/1.0") // user agent - сервис health checker

	resp, err := c.client.Do(req)
	if err != nil {
		// сетевая ошибка: таймаут, не удалось подключиться или другие
		return fmt.Errorf("health check не удался для %s: %w", target, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil // здоров
	}

	// любой другой статус считается ошибкой
	return fmt.Errorf("health check failed for %s: unexpected status code %d", target, resp.StatusCode)
}
