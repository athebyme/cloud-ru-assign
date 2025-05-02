package healthcheck

import (
	"cloud-ru-assign/internal/core/ports"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type HTTPChecker struct {
	client  *http.Client
	timeout time.Duration
	path    string
}

func NewHTTPChecker(timeout time.Duration, path string) ports.HealthChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DisableKeepAlives: true,
			},
		},
		timeout: timeout,
		path:    path,
	}
}

func (c *HTTPChecker) Check(target *url.URL) error {
	checkURL := target.String()
	if c.path != "" {
		checkURL = target.JoinPath(c.path).String()
	}
	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request for %s: %w", target, err)
	}
	req.Header.Set("User-Agent", "LoadBalancer-HealthChecker/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed for %s: %w", target, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	return fmt.Errorf("health check failed for %s: unexpected status code %d", target, resp.StatusCode)
}
