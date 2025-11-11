package vllm

import (
	"context"
	"errors"
	"testing"
)

func TestMockClient_Health(t *testing.T) {
	mock := NewMockClient()

	ctx := context.Background()
	healthy, err := mock.Health(ctx, "vllm-test", 8000)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !healthy {
		t.Error("expected healthy=true")
	}

	if len(mock.HealthCalls) != 1 {
		t.Errorf("expected 1 health call, got %d", len(mock.HealthCalls))
	}

	call := mock.HealthCalls[0]
	if call.Host != "vllm-test" || call.Port != 8000 {
		t.Errorf("expected call to vllm-test:8000, got %s:%d", call.Host, call.Port)
	}
}

func TestMockClient_HealthCustomFunc(t *testing.T) {
	mock := NewMockClient()
	mock.HealthFunc = func(ctx context.Context, host string, port int) (bool, error) {
		return false, errors.New("custom error")
	}

	ctx := context.Background()
	healthy, err := mock.Health(ctx, "vllm-test", 8000)

	if err == nil {
		t.Error("expected error")
	}

	if healthy {
		t.Error("expected healthy=false")
	}
}

func TestMockClient_IsSleeping(t *testing.T) {
	mock := NewMockClient()

	ctx := context.Background()
	sleeping, err := mock.IsSleeping(ctx, "vllm-test", 8000)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if sleeping {
		t.Error("expected sleeping=false by default")
	}

	if len(mock.IsSleepingCalls) != 1 {
		t.Errorf("expected 1 is_sleeping call, got %d", len(mock.IsSleepingCalls))
	}
}

func TestMockClient_IsSleepingCustomFunc(t *testing.T) {
	mock := NewMockClient()
	mock.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-sleeping" {
			return true, nil
		}
		return false, nil
	}

	ctx := context.Background()

	sleeping, _ := mock.IsSleeping(ctx, "vllm-sleeping", 8000)
	if !sleeping {
		t.Error("expected vllm-sleeping to be sleeping")
	}

	sleeping, _ = mock.IsSleeping(ctx, "vllm-awake", 8000)
	if sleeping {
		t.Error("expected vllm-awake to be awake")
	}
}

func TestMockClient_Sleep(t *testing.T) {
	mock := NewMockClient()

	ctx := context.Background()
	err := mock.Sleep(ctx, "vllm-test", 8000, 1)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(mock.SleepCalls) != 1 {
		t.Errorf("expected 1 sleep call, got %d", len(mock.SleepCalls))
	}

	call := mock.SleepCalls[0]
	if call.Host != "vllm-test" || call.Port != 8000 || call.Level != 1 {
		t.Errorf("expected call to vllm-test:8000 level 1, got %s:%d level %d", call.Host, call.Port, call.Level)
	}
}

func TestMockClient_SleepCustomFunc(t *testing.T) {
	mock := NewMockClient()
	mock.SleepFunc = func(ctx context.Context, host string, port int, level int) error {
		return errors.New("sleep failed")
	}

	ctx := context.Background()
	err := mock.Sleep(ctx, "vllm-test", 8000, 1)

	if err == nil {
		t.Error("expected error")
	}
}

func TestMockClient_WakeUp(t *testing.T) {
	mock := NewMockClient()

	ctx := context.Background()
	err := mock.WakeUp(ctx, "vllm-test", 8000)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(mock.WakeUpCalls) != 1 {
		t.Errorf("expected 1 wake_up call, got %d", len(mock.WakeUpCalls))
	}

	call := mock.WakeUpCalls[0]
	if call.Host != "vllm-test" || call.Port != 8000 {
		t.Errorf("expected call to vllm-test:8000, got %s:%d", call.Host, call.Port)
	}
}

func TestMockClient_WakeUpCustomFunc(t *testing.T) {
	mock := NewMockClient()
	mock.WakeUpFunc = func(ctx context.Context, host string, port int) error {
		return errors.New("wake_up failed")
	}

	ctx := context.Background()
	err := mock.WakeUp(ctx, "vllm-test", 8000)

	if err == nil {
		t.Error("expected error")
	}
}

func TestMockClient_Reset(t *testing.T) {
	mock := NewMockClient()

	ctx := context.Background()
	mock.Health(ctx, "test", 8000)
	mock.IsSleeping(ctx, "test", 8000)
	mock.Sleep(ctx, "test", 8000, 1)
	mock.WakeUp(ctx, "test", 8000)

	if len(mock.HealthCalls) != 1 || len(mock.IsSleepingCalls) != 1 ||
		len(mock.SleepCalls) != 1 || len(mock.WakeUpCalls) != 1 {
		t.Error("expected all call slices to have 1 entry")
	}

	mock.Reset()

	if len(mock.HealthCalls) != 0 || len(mock.IsSleepingCalls) != 0 ||
		len(mock.SleepCalls) != 0 || len(mock.WakeUpCalls) != 0 {
		t.Error("expected all call slices to be empty after reset")
	}
}

func TestMockClient_ThreadSafety(t *testing.T) {
	mock := NewMockClient()
	ctx := context.Background()

	// Run multiple goroutines concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			mock.Health(ctx, "test", 8000)
			mock.IsSleeping(ctx, "test", 8000)
			mock.Sleep(ctx, "test", 8000, 1)
			mock.WakeUp(ctx, "test", 8000)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all calls were recorded
	if len(mock.HealthCalls) != 10 {
		t.Errorf("expected 10 health calls, got %d", len(mock.HealthCalls))
	}
	if len(mock.IsSleepingCalls) != 10 {
		t.Errorf("expected 10 is_sleeping calls, got %d", len(mock.IsSleepingCalls))
	}
	if len(mock.SleepCalls) != 10 {
		t.Errorf("expected 10 sleep calls, got %d", len(mock.SleepCalls))
	}
	if len(mock.WakeUpCalls) != 10 {
		t.Errorf("expected 10 wake_up calls, got %d", len(mock.WakeUpCalls))
	}
}
