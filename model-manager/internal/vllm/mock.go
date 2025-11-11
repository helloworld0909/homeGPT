package vllm

import (
	"context"
	"sync"
)

// MockClient is a mock implementation of VLLMClient for testing
type MockClient struct {
	mu sync.Mutex

	// Mock state
	HealthFunc     func(ctx context.Context, host string, port int) (bool, error)
	IsSleepingFunc func(ctx context.Context, host string, port int) (bool, error)
	SleepFunc      func(ctx context.Context, host string, port int, level int) error
	WakeUpFunc     func(ctx context.Context, host string, port int) error

	// Call tracking
	HealthCalls     []HealthCall
	IsSleepingCalls []IsSleepingCall
	SleepCalls      []SleepCall
	WakeUpCalls     []WakeUpCall
}

type HealthCall struct {
	Host string
	Port int
}

type IsSleepingCall struct {
	Host string
	Port int
}

type SleepCall struct {
	Host  string
	Port  int
	Level int
}

type WakeUpCall struct {
	Host string
	Port int
}

// NewMockClient creates a new mock vLLM client
func NewMockClient() *MockClient {
	return &MockClient{
		HealthCalls:     make([]HealthCall, 0),
		IsSleepingCalls: make([]IsSleepingCall, 0),
		SleepCalls:      make([]SleepCall, 0),
		WakeUpCalls:     make([]WakeUpCall, 0),
	}
}

func (m *MockClient) Health(ctx context.Context, host string, port int) (bool, error) {
	m.mu.Lock()
	m.HealthCalls = append(m.HealthCalls, HealthCall{Host: host, Port: port})
	m.mu.Unlock()

	if m.HealthFunc != nil {
		return m.HealthFunc(ctx, host, port)
	}
	return true, nil
}

func (m *MockClient) IsSleeping(ctx context.Context, host string, port int) (bool, error) {
	m.mu.Lock()
	m.IsSleepingCalls = append(m.IsSleepingCalls, IsSleepingCall{Host: host, Port: port})
	m.mu.Unlock()

	if m.IsSleepingFunc != nil {
		return m.IsSleepingFunc(ctx, host, port)
	}
	return false, nil
}

func (m *MockClient) Sleep(ctx context.Context, host string, port int, level int) error {
	m.mu.Lock()
	m.SleepCalls = append(m.SleepCalls, SleepCall{Host: host, Port: port, Level: level})
	m.mu.Unlock()

	if m.SleepFunc != nil {
		return m.SleepFunc(ctx, host, port, level)
	}
	return nil
}

func (m *MockClient) WakeUp(ctx context.Context, host string, port int) error {
	m.mu.Lock()
	m.WakeUpCalls = append(m.WakeUpCalls, WakeUpCall{Host: host, Port: port})
	m.mu.Unlock()

	if m.WakeUpFunc != nil {
		return m.WakeUpFunc(ctx, host, port)
	}
	return nil
}

// Reset clears all call tracking
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.HealthCalls = make([]HealthCall, 0)
	m.IsSleepingCalls = make([]IsSleepingCall, 0)
	m.SleepCalls = make([]SleepCall, 0)
	m.WakeUpCalls = make([]WakeUpCall, 0)
}

// Ensure MockClient implements VLLMClient interface
var _ VLLMClient = (*MockClient)(nil)
