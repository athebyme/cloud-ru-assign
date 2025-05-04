package app

import (
	"errors"
	"github.com/golang/mock/gomock"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/athebyme/cloud-ru-assign/internal/core/app"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"github.com/athebyme/cloud-ru-assign/internal/test/mocks"
)

func TestLoadBalancerService_HandleRequest_Success(t *testing.T) {
	// arrange
	mockRepo := mocks.NewMockBackendRepository(t)
	mockForwarder := mocks.NewMockForwarder(t)
	mockLogger := mocks.NewMockLogger(t)

	testBackend := &balancer.Backend{URL: parseURL("http://test-backend")}

	mockRepo.EXPECT().GetNextHealthyBackend().Return(testBackend, true)
	mockForwarder.EXPECT().Forward(mocks.MockAny(), mocks.MockAny(), testBackend).Return(nil)
	mockLogger.EXPECT().With("service", "LoadBalancerService").Return(mockLogger)
	mockLogger.EXPECT().Info("Request forwarded successfully", mocks.MockAny()).Return()

	service := app.NewLoadBalancerService(mockRepo, mockForwarder, mockLogger)

	// act
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	service.HandleRequest(rec, req)

	// assert
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestLoadBalancerService_HandleRequest_NoHealthyBackends(t *testing.T) {
	// arrange
	mockRepo := mocks.NewMockBackendRepository(t)
	mockForwarder := mocks.NewMockForwarder(t)
	mockLogger := mocks.NewMockLogger(t)

	mockRepo.EXPECT().GetNextHealthyBackend().Return(nil, false)
	mockLogger.EXPECT().With(gomock.Any()).AnyTimes().Return(mockLogger)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes()

	service := app.NewLoadBalancerService(mockRepo, mockForwarder, mockLogger)

	// act
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	service.HandleRequest(rec, req)

	// assert
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestLoadBalancerService_HandleRequest_ForwardingError(t *testing.T) {
	// arrange
	mockRepo := mocks.NewMockBackendRepository(t)
	mockForwarder := mocks.NewMockForwarder(t)
	mockLogger := mocks.NewMockLogger(t)

	backend1 := &balancer.Backend{URL: parseURL("http://backend1")}
	backend2 := &balancer.Backend{URL: parseURL("http://backend2")}

	mockRepo.EXPECT().GetNextHealthyBackend().Return(backend1, true)
	mockForwarder.EXPECT().Forward(mocks.MockAny(), mocks.MockAny(), backend1).Return(errors.New("forwarding failed"))
	mockRepo.EXPECT().MarkBackendStatus(backend1.URL, false)

	mockRepo.EXPECT().GetNextHealthyBackend().Return(backend2, true)
	mockForwarder.EXPECT().Forward(mocks.MockAny(), mocks.MockAny(), backend2).Return(nil)

	mockLogger.EXPECT().With(gomock.Any()).AnyTimes().Return(mockLogger)
	mockLogger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes()
	mockLogger.EXPECT().Warn(gomock.Any(), gomock.Any()).AnyTimes()

	service := app.NewLoadBalancerService(mockRepo, mockForwarder, mockLogger)

	// act
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	service.HandleRequest(rec, req)

	// assert
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d after retry, got %d", http.StatusOK, rec.Code)
	}
}

func parseURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}
