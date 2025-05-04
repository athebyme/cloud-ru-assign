package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/athebyme/cloud-ru-assign/internal/core/app"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"github.com/athebyme/cloud-ru-assign/internal/test/mocks"
	"github.com/golang/mock/gomock"
)

func TestLoadBalancerService_HandleRequest_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBackendRepository(ctrl)
	mockForwarder := mocks.NewMockForwarder(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	testBackend := &balancer.Backend{URL: parseURL("http://test-backend")}

	mockLogger.EXPECT().With("service", "LoadBalancerService").Return(mockLogger)

	service := app.NewLoadBalancerService(mockRepo, mockForwarder, mockLogger)

	mockLogger.EXPECT().With(
		"method", gomock.Any(),
		"uri", gomock.Any(),
		"remote_addr", gomock.Any(),
	).Return(mockLogger)

	mockLogger.EXPECT().Info("начало обработки входящего запроса").Return()

	mockRepo.EXPECT().GetNextHealthyBackend().Return(testBackend, true)

	mockLogger.EXPECT().With("attempt", 1).Return(mockLogger)
	mockLogger.EXPECT().With("backend_url", "http://test-backend").Return(mockLogger)

	mockLogger.EXPECT().Info("попытка перенаправления запроса на бэкенд").Return()
	mockForwarder.EXPECT().Forward(gomock.Any(), gomock.Any(), testBackend).Return(nil)
	mockLogger.EXPECT().Info("Request forwarded successfully", gomock.Any()).Return()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	service.HandleRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestLoadBalancerService_HandleRequest_NoHealthyBackends(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBackendRepository(ctrl)
	mockForwarder := mocks.NewMockForwarder(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().With("service", "LoadBalancerService").Return(mockLogger)

	service := app.NewLoadBalancerService(mockRepo, mockForwarder, mockLogger)

	mockLogger.EXPECT().With(
		"method", gomock.Any(),
		"uri", gomock.Any(),
		"remote_addr", gomock.Any(),
	).Return(mockLogger)

	mockLogger.EXPECT().Info("начало обработки входящего запроса").Return()

	mockLogger.EXPECT().With("attempt", 1).Return(mockLogger)
	mockRepo.EXPECT().GetNextHealthyBackend().Return(nil, false)
	mockLogger.EXPECT().Warn("нет доступных здоровых бэкендов").Return()

	mockLogger.EXPECT().Error("Failed to handle request after all retries", gomock.Any()).Return()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	service.HandleRequest(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestLoadBalancerService_HandleRequest_ForwardingErrorWithRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBackendRepository(ctrl)
	mockForwarder := mocks.NewMockForwarder(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	backend1 := &balancer.Backend{URL: parseURL("http://backend1")}
	backend2 := &balancer.Backend{URL: parseURL("http://backend2")}

	// попытка (неудачная)
	mockRepo.EXPECT().GetNextHealthyBackend().Return(backend1, true).Times(1)
	mockForwarder.EXPECT().Forward(gomock.Any(), gomock.Any(), backend1).Return(errors.New("forwarding failed")).Times(1)
	mockRepo.EXPECT().MarkBackendStatus(backend1.URL, false).Times(1)

	// попытка (успешная)
	mockRepo.EXPECT().GetNextHealthyBackend().Return(backend2, true).Times(1)
	mockForwarder.EXPECT().Forward(gomock.Any(), gomock.Any(), backend2).Return(nil).Times(1)

	mockLogger.EXPECT().With("service", "LoadBalancerService").Return(mockLogger)
	mockLogger.EXPECT().With(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()

	service := app.NewLoadBalancerService(mockRepo, mockForwarder, mockLogger)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	service.HandleRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d after retry, got %d", http.StatusOK, rec.Code)
	}
}

func TestLoadBalancerService_HandleRequest_MaxRetriesExhausted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockBackendRepository(ctrl)
	mockForwarder := mocks.NewMockForwarder(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	backend := &balancer.Backend{URL: parseURL("http://backend")}

	//  попытки неудачные
	for i := 0; i < 3; i++ {
		mockRepo.EXPECT().GetNextHealthyBackend().Return(backend, true)
		mockForwarder.EXPECT().Forward(gomock.Any(), gomock.Any(), backend).Return(errors.New("forwarding failed"))
		mockRepo.EXPECT().MarkBackendStatus(backend.URL, false)
	}

	mockLogger.EXPECT().With("service", "LoadBalancerService").Return(mockLogger)
	mockLogger.EXPECT().With(gomock.Any()).Return(mockLogger).AnyTimes()
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	service := app.NewLoadBalancerService(mockRepo, mockForwarder, mockLogger)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	service.HandleRequest(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d after exhausting retries, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func parseURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}
