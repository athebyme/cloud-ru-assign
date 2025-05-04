package mocks

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/athebyme/cloud-ru-assign/internal/core/domain/balancer"
	"github.com/athebyme/cloud-ru-assign/internal/core/domain/ratelimit"
	"github.com/athebyme/cloud-ru-assign/internal/core/ports"
	"github.com/golang/mock/gomock"
)

func NewMockBackendRepository(t *testing.T) *MockBackendRepository {
	ctrl := gomock.NewController(t)
	return &MockBackendRepository{ctrl: ctrl}
}

type MockBackendRepository struct {
	ctrl *gomock.Controller
}

func (m *MockBackendRepository) GetBackends() []*balancer.Backend {
	ret := m.ctrl.Call(m, "GetBackends")
	if ret[0] == nil {
		return nil
	}
	return ret[0].([]*balancer.Backend)
}

func (m *MockBackendRepository) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	m.ctrl.Call(m, "MarkBackendStatus", backendUrl, alive)
}

func (m *MockBackendRepository) GetNextHealthyBackend() (*balancer.Backend, bool) {
	ret := m.ctrl.Call(m, "GetNextHealthyBackend")
	if ret[0] == nil {
		return nil, ret[1].(bool)
	}
	return ret[0].(*balancer.Backend), ret[1].(bool)
}

func (m *MockBackendRepository) SetStrategy(strategy string) error {
	ret := m.ctrl.Call(m, "SetStrategy", strategy)
	if ret[0] == nil {
		return nil
	}
	return ret[0].(error)
}

func (m *MockBackendRepository) GetActiveConnections(backend *balancer.Backend) int {
	ret := m.ctrl.Call(m, "GetActiveConnections", backend)
	return ret[0].(int)
}

func (m *MockBackendRepository) IncrementConnections(backend *balancer.Backend) {
	m.ctrl.Call(m, "IncrementConnections", backend)
}

func (m *MockBackendRepository) DecrementConnections(backend *balancer.Backend) {
	m.ctrl.Call(m, "DecrementConnections", backend)
}

func (m *MockBackendRepository) EXPECT() *MockBackendRepositoryExpect {
	return &MockBackendRepositoryExpect{m}
}

type MockBackendRepositoryExpect struct {
	*MockBackendRepository
}

func (m *MockBackendRepositoryExpect) GetNextHealthyBackend() *MockBackendRepositoryGetNextHealthyBackendCall {
	call := m.ctrl.RecordCall(m, "GetNextHealthyBackend")
	return &MockBackendRepositoryGetNextHealthyBackendCall{Call: call}
}

type MockBackendRepositoryGetNextHealthyBackendCall struct {
	*gomock.Call
}

func (c *MockBackendRepositoryGetNextHealthyBackendCall) Return(backend *balancer.Backend, found bool) *MockBackendRepositoryGetNextHealthyBackendCall {
	c.Call = c.Call.Return(backend, found)
	return c
}

func (m *MockBackendRepositoryExpect) MarkBackendStatus(backendUrl *url.URL, alive bool) *MockBackendRepositoryMarkBackendStatusCall {
	call := m.ctrl.RecordCall(m, "MarkBackendStatus", backendUrl, alive)
	return &MockBackendRepositoryMarkBackendStatusCall{Call: call}
}

type MockBackendRepositoryMarkBackendStatusCall struct {
	*gomock.Call
}

func NewMockForwarder(t *testing.T) *MockForwarder {
	ctrl := gomock.NewController(t)
	return &MockForwarder{ctrl: ctrl}
}

type MockForwarder struct {
	ctrl *gomock.Controller
}

func (m *MockForwarder) Forward(w http.ResponseWriter, r *http.Request, target *balancer.Backend) error {
	ret := m.ctrl.Call(m, "Forward", w, r, target)
	if ret[0] == nil {
		return nil
	}
	return ret[0].(error)
}

func (m *MockForwarder) EXPECT() *MockForwarderExpect {
	return &MockForwarderExpect{m}
}

type MockForwarderExpect struct {
	*MockForwarder
}

func (m *MockForwarderExpect) Forward(w, r, target interface{}) *MockForwarderForwardCall {
	call := m.ctrl.RecordCall(m, "Forward", w, r, target)
	return &MockForwarderForwardCall{Call: call}
}

type MockForwarderForwardCall struct {
	*gomock.Call
}

func (c *MockForwarderForwardCall) Return(err error) *MockForwarderForwardCall {
	c.Call = c.Call.Return(err)
	return c
}

func NewMockLogger(t *testing.T) *MockLogger {
	ctrl := gomock.NewController(t)
	return &MockLogger{ctrl: ctrl}
}

type MockLogger struct {
	ctrl *gomock.Controller
}

func (m *MockLogger) Debug(msg string, args ...any) {
	varArgs := []interface{}{msg}
	for _, a := range args {
		varArgs = append(varArgs, a)
	}
	m.ctrl.Call(m, "Debug", varArgs...)
}

func (m *MockLogger) Info(msg string, args ...any) {
	varArgs := []interface{}{msg}
	for _, a := range args {
		varArgs = append(varArgs, a)
	}
	m.ctrl.Call(m, "Info", varArgs...)
}

func (m *MockLogger) Warn(msg string, args ...any) {
	varArgs := []interface{}{msg}
	for _, a := range args {
		varArgs = append(varArgs, a)
	}
	m.ctrl.Call(m, "Warn", varArgs...)
}

func (m *MockLogger) Error(msg string, args ...any) {
	varArgs := []interface{}{msg}
	for _, a := range args {
		varArgs = append(varArgs, a)
	}
	m.ctrl.Call(m, "Error", varArgs...)
}

func (m *MockLogger) With(args ...any) ports.Logger {
	ret := m.ctrl.Call(m, "With", args)
	return ret[0].(ports.Logger)
}

func (m *MockLogger) EXPECT() *MockLoggerExpect {
	return &MockLoggerExpect{m}
}

type MockLoggerExpect struct {
	*MockLogger
}

func (m *MockLoggerExpect) With(args ...interface{}) *MockLoggerWithCall {
	call := m.ctrl.RecordCall(m, "With", args...)
	return &MockLoggerWithCall{Call: call}
}

type MockLoggerWithCall struct {
	*gomock.Call
}

func (c *MockLoggerWithCall) Return(logger ports.Logger) *MockLoggerWithCall {
	c.Call = c.Call.Return(logger)
	return c
}

func (m *MockLoggerExpect) Info(msg interface{}, args ...interface{}) *MockLoggerInfoCall {
	varArgs := append([]interface{}{msg}, args...)
	call := m.ctrl.RecordCall(m, "Info", varArgs...)
	return &MockLoggerInfoCall{Call: call}
}

type MockLoggerInfoCall struct {
	*gomock.Call
}

func (m *MockLoggerExpect) Error(msg interface{}, args ...interface{}) *MockLoggerErrorCall {
	varArgs := append([]interface{}{msg}, args...)
	call := m.ctrl.RecordCall(m, "Error", varArgs...)
	return &MockLoggerErrorCall{Call: call}
}

type MockLoggerErrorCall struct {
	*gomock.Call
}

func (m *MockLoggerExpect) Warn(msg interface{}, args ...interface{}) *MockLoggerWarnCall {
	varArgs := append([]interface{}{msg}, args...)
	call := m.ctrl.RecordCall(m, "Warn", varArgs...)
	return &MockLoggerWarnCall{Call: call}
}

type MockLoggerWarnCall struct {
	*gomock.Call
}

// MockRateLimiter создает мок для RateLimiter
func NewMockRateLimiter(t *testing.T) *MockRateLimiter {
	ctrl := gomock.NewController(t)
	return &MockRateLimiter{ctrl: ctrl}
}

type MockRateLimiter struct {
	ctrl *gomock.Controller
}

func (m *MockRateLimiter) Allow(clientID string) bool {
	ret := m.ctrl.Call(m, "Allow", clientID)
	return ret[0].(bool)
}

func (m *MockRateLimiter) SetRateLimit(clientID string, settings *ratelimit.RateLimitSettings) error {
	ret := m.ctrl.Call(m, "SetRateLimit", clientID, settings)
	if ret[0] == nil {
		return nil
	}
	return ret[0].(error)
}

func (m *MockRateLimiter) RemoveRateLimit(clientID string) {
	m.ctrl.Call(m, "RemoveRateLimit", clientID)
}

func (m *MockRateLimiter) Stop() {
	m.ctrl.Call(m, "Stop")
}

func (m *MockRateLimiter) EXPECT() *MockRateLimiterExpect {
	return &MockRateLimiterExpect{m}
}

type MockRateLimiterExpect struct {
	*MockRateLimiter
}

func (m *MockRateLimiterExpect) SetRateLimit(clientID interface{}, settings interface{}) *MockRateLimiterSetRateLimitCall {
	call := m.ctrl.RecordCall(m, "SetRateLimit", clientID, settings)
	return &MockRateLimiterSetRateLimitCall{Call: call}
}

type MockRateLimiterSetRateLimitCall struct {
	*gomock.Call
}

func (c *MockRateLimiterSetRateLimitCall) Return(err error) *MockRateLimiterSetRateLimitCall {
	c.Call = c.Call.Return(err)
	return c
}

func MockAny() gomock.Matcher {
	return gomock.Any()
}
