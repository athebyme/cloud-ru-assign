package app

import (
	"testing"

	"github.com/athebyme/cloud-ru-assign/internal/core/app"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/test/mocks"
	"github.com/golang/mock/gomock"
)

func TestRateLimitService_CreateOrUpdateClient_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLimiter := mocks.NewMockRateLimiter(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	settings := &ratelimit.RateLimitSettings{
		ClientID:      "test-client",
		Capacity:      100,
		RatePerSecond: 10,
	}

	mockLogger.EXPECT().With("service", "RateLimitService").Return(mockLogger)
	mockLimiter.EXPECT().SetRateLimit("test-client", settings).Return(nil)
	mockLogger.EXPECT().Info("Rate limit settings updated", "client", "test-client")

	service := app.NewRateLimitService(mockLimiter, mockLogger)

	err := service.CreateOrUpdateClient(settings)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRateLimitService_CreateOrUpdateClient_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name     string
		settings *ratelimit.RateLimitSettings
		expected string
	}{
		{
			name: "empty client ID",
			settings: &ratelimit.RateLimitSettings{
				ClientID:      "",
				Capacity:      100,
				RatePerSecond: 10,
			},
			expected: "client_id не может быть пустым",
		},
		{
			name: "zero capacity",
			settings: &ratelimit.RateLimitSettings{
				ClientID:      "test",
				Capacity:      0,
				RatePerSecond: 10,
			},
			expected: "capacity должно быть больше 0",
		},
		{
			name: "zero rate",
			settings: &ratelimit.RateLimitSettings{
				ClientID:      "test",
				Capacity:      100,
				RatePerSecond: 0,
			},
			expected: "rate_per_second должно быть больше 0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockLimiter := mocks.NewMockRateLimiter(ctrl)
			mockLogger := mocks.NewMockLogger(ctrl)

			mockLogger.EXPECT().With("service", "RateLimitService").Return(mockLogger)

			service := app.NewRateLimitService(mockLimiter, mockLogger)

			err := service.CreateOrUpdateClient(tc.settings)

			if err == nil || err.Error() != tc.expected {
				t.Errorf("expected error %q, got %v", tc.expected, err)
			}
		})
	}
}

func TestRateLimitService_RemoveClient_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLimiter := mocks.NewMockRateLimiter(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().With("service", "RateLimitService").Return(mockLogger)

	service := app.NewRateLimitService(mockLimiter, mockLogger)

	settings := &ratelimit.RateLimitSettings{
		ClientID:      "test-client",
		Capacity:      100,
		RatePerSecond: 10,
	}

	mockLimiter.EXPECT().SetRateLimit("test-client", settings).Return(nil)
	mockLogger.EXPECT().Info("Rate limit settings updated", "client", "test-client")

	err := service.CreateOrUpdateClient(settings)
	if err != nil {
		t.Fatal(err)
	}

	// Удаляем клиента
	mockLimiter.EXPECT().RemoveRateLimit("test-client")
	mockLogger.EXPECT().Info("Client removed", "client", "test-client")

	err = service.RemoveClient("test-client")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRateLimitService_RemoveClient_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLimiter := mocks.NewMockRateLimiter(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().With("service", "RateLimitService").Return(mockLogger)

	service := app.NewRateLimitService(mockLimiter, mockLogger)

	err := service.RemoveClient("non-existent")

	if err == nil || err.Error() != "client not found" {
		t.Errorf("expected 'client not found' error, got %v", err)
	}
}

func TestRateLimitService_GetClientSettings_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLimiter := mocks.NewMockRateLimiter(ctrl)
	mockLogger := mocks.NewMockLogger(ctrl)

	mockLogger.EXPECT().With("service", "RateLimitService").Return(mockLogger)

	service := app.NewRateLimitService(mockLimiter, mockLogger)

	expectedSettings := &ratelimit.RateLimitSettings{
		ClientID:      "test-client",
		Capacity:      100,
		RatePerSecond: 10,
	}

	mockLimiter.EXPECT().SetRateLimit("test-client", expectedSettings).Return(nil)
	mockLogger.EXPECT().Info("Rate limit settings updated", "client", "test-client")

	err := service.CreateOrUpdateClient(expectedSettings)
	if err != nil {
		t.Fatal(err)
	}

	settings, err := service.GetClientSettings("test-client")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if settings.ClientID != expectedSettings.ClientID ||
		settings.Capacity != expectedSettings.Capacity ||
		settings.RatePerSecond != expectedSettings.RatePerSecond {
		t.Errorf("settings mismatch")
	}
}
