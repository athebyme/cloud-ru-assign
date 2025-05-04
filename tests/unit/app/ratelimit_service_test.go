package app

import (
	"testing"

	"github.com/athebyme/cloud-ru-assign/internal/core/app"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/test/mocks"
)

func TestRateLimitService_CreateOrUpdateClient_Success(t *testing.T) {
	// arrange
	mockLimiter := mocks.NewMockRateLimiter(t)
	mockLogger := mocks.NewMockLogger(t)

	settings := &ratelimit.RateLimitSettings{
		ClientID:      "test-client",
		Capacity:      100,
		RatePerSecond: 10,
	}

	mockLimiter.EXPECT().SetRateLimit("test-client", settings).Return(nil)
	mockLogger.EXPECT().With("service", "RateLimitService").Return(mockLogger)
	mockLogger.EXPECT().Info("Rate limit settings updated", "client", "test-client")

	service := app.NewRateLimitService(mockLimiter, mockLogger)

	// act
	err := service.CreateOrUpdateClient(settings)

	// assert
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
			mockLimiter := mocks.NewMockRateLimiter(t)
			mockLogger := mocks.NewMockLogger(t)

			mockLogger.EXPECT().With("service", "RateLimitService").Return(mockLogger)

			service := app.NewRateLimitService(mockLimiter, mockLogger)

			err := service.CreateOrUpdateClient(tc.settings)

			if err == nil || err.Error() != tc.expected {
				t.Errorf("expected error %q, got %v", tc.expected, err)
			}
		})
	}
}
