package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

const baseURL = "http://loadbalancer:8080"

func TestE2E_LoadBalancerFunctionality(t *testing.T) {
	t.Run("LoadBalancer responds", func(t *testing.T) {
		resp, err := http.Get(baseURL)
		if err != nil {
			t.Fatalf("Failed to connect to loadbalancer: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Round-robin distribution", func(t *testing.T) {
		responses := make(map[string]int)

		for i := 0; i < 10; i++ {
			resp, err := http.Get(baseURL)
			if err != nil {
				t.Fatalf("Request %d failed: %v", i, err)
			}

			buf := new(bytes.Buffer)
			buf.ReadFrom(resp.Body)
			body := buf.String()
			responses[body]++
			resp.Body.Close()
		}

		// Проверяем, что запросы распределены между бэкендами
		if len(responses) != 2 {
			t.Errorf("Expected 2 backends, got %d", len(responses))
		}

		for backend, count := range responses {
			t.Logf("Backend '%s': %d requests", backend, count)
		}
	})
}

func TestE2E_RateLimiting(t *testing.T) {
	t.Run("Create rate limit", func(t *testing.T) {
		settings := map[string]interface{}{
			"client_id":       "test-client",
			"capacity":        5,
			"rate_per_second": 1,
		}

		jsonData, _ := json.Marshal(settings)
		url := baseURL + "/api/v1/ratelimit/clients"

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatalf("Failed to create rate limit: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}
	})

	t.Run("Rate limit enforcement", func(t *testing.T) {
		// Тестируем с API ключом
		client := &http.Client{}

		// Первые 5 запросов должны пройти
		for i := 0; i < 5; i++ {
			req, _ := http.NewRequest("GET", baseURL, nil)
			req.Header.Set("X-API-Key", "test-client")

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Request %d failed: %v", i, err)
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Request %d: expected 200, got %d", i, resp.StatusCode)
			}
		}

		// 6-й запрос должен быть отклонен
		req, _ := http.NewRequest("GET", baseURL, nil)
		req.Header.Set("X-API-Key", "test-client")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send 6th request: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusTooManyRequests {
			t.Errorf("6th request: expected 429, got %d", resp.StatusCode)
		}
	})

	t.Run("List and delete clients", func(t *testing.T) {
		// Получаем список клиентов
		resp, err := http.Get(baseURL + "/api/v1/ratelimit/clients")
		if err != nil {
			t.Fatalf("Failed to list clients: %v", err)
		}

		var clients []interface{}
		json.NewDecoder(resp.Body).Decode(&clients)
		resp.Body.Close()

		if len(clients) == 0 {
			t.Error("Expected at least one client")
		}

		// Удаляем клиента
		client := &http.Client{}
		req, _ := http.NewRequest("DELETE", baseURL+"/api/v1/ratelimit/clients/test-client", nil)

		deleteResp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to delete client: %v", err)
		}
		defer deleteResp.Body.Close()

		if deleteResp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected status 204, got %d", deleteResp.StatusCode)
		}
	})
}

func TestE2E_HealthCheck(t *testing.T) {
	t.Run("Backend health monitoring", func(t *testing.T) {
		// Имитируем падение бэкенда (останавливаем контейнер)
		stopBackend(t, "backend1")

		// Даем время на обнаружение сбоя
		time.Sleep(15 * time.Second)

		// Проверяем, что все запросы идут на второй бэкенд
		responses := make(map[string]int)
		for i := 0; i < 10; i++ {
			resp, err := http.Get(baseURL)
			if err != nil {
				t.Fatalf("Request %d failed: %v", i, err)
			}

			buf := new(bytes.Buffer)
			buf.ReadFrom(resp.Body)
			body := buf.String()
			responses[body]++
			resp.Body.Close()
		}

		if len(responses) != 1 {
			t.Errorf("Expected 1 backend, got %d", len(responses))
		}

		// Восстанавливаем бэкенд
		startBackend(t, "backend1")

		// Даем время на восстановление
		time.Sleep(15 * time.Second)

		// Проверяем, что балансировка восстановилась
		responses = make(map[string]int)
		for i := 0; i < 10; i++ {
			resp, _ := http.Get(baseURL)
			buf := new(bytes.Buffer)
			buf.ReadFrom(resp.Body)
			body := buf.String()
			responses[body]++
			resp.Body.Close()
		}

		if len(responses) != 2 {
			t.Errorf("Expected 2 backends after recovery, got %d", len(responses))
		}
	})
}

func TestE2E_GracefulShutdown(t *testing.T) {
	t.Run("Shutdown with active requests", func(t *testing.T) {
		// Запускаем 5 долгих запросов
		for i := 0; i < 5; i++ {
			go func() {
				http.Get(baseURL + "/slow")
			}()
		}

		time.Sleep(100 * time.Millisecond)

		// Отправляем сигнал SIGTERM балансировщику
		sendSIGTERM(t, "loadbalancer")

		// Проверяем, что новые запросы не принимаются
		resp, err := http.Get(baseURL)
		if err == nil {
			resp.Body.Close()
			t.Error("Expected connection error, got response")
		}

		// Проверяем, что контейнер завершился
		checkContainerStopped(t, "loadbalancer", 20*time.Second)
	})
}

func sendSIGTERM(t *testing.T, container string) {
	// Тут логика для отправки SIGTERM в docker контейнер
	// docker.KillContainer(container, "SIGTERM")
}

func stopBackend(t *testing.T, container string) {
	// Логика остановки контейнера
	// docker.StopContainer(container)
}

func startBackend(t *testing.T, container string) {
	// Логика запуска контейнера
	// docker.StartContainer(container)
}

func checkContainerStopped(t *testing.T, container string, timeout time.Duration) {
	// Проверка, что контейнер остановлен
	// docker.WaitForContainerStop(container, timeout)
}
