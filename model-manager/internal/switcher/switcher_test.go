package switcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zheng/homeGPT/internal/system"
	"github.com/zheng/homeGPT/internal/vllm"
	"github.com/zheng/homeGPT/pkg/models"
)

func TestNew(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{
				ID:            "model-a",
				ContainerName: "vllm-a",
				Port:          8000,
				HostPort:      8001,
				StartupMode:   models.StartupActive,
				GPUMemoryGB:   16.0,
			},
			{
				ID:            "model-b",
				ContainerName: "vllm-b",
				Port:          8000,
				HostPort:      8002,
				StartupMode:   models.StartupSleep,
				GPUMemoryGB:   24.0,
			},
		},
	}

	mockClient := vllm.NewMockClient()
	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))

	if s == nil {
		t.Fatal("expected non-nil switcher")
	}

	if s.activeModel != "model-a" {
		t.Errorf("expected active model 'model-a', got '%s'", s.activeModel)
	}

	if len(s.models) != 2 {
		t.Errorf("expected 2 models, got %d", len(s.models))
	}

	// Check default model is active
	if s.models["model-a"].GetStatus() != models.StatusActive {
		t.Errorf("expected model-a to be active, got %s", s.models["model-a"].GetStatus())
	}

	// Check non-default model is sleeping
	if s.models["model-b"].GetStatus() != models.StatusSleeping {
		t.Errorf("expected model-b to be sleeping, got %s", s.models["model-b"].GetStatus())
	}
}

func TestGetModels(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{
				ID:            "model-a",
				ContainerName: "vllm-a",
				Port:          8000,
				HostPort:      8001,
				StartupMode:   models.StartupActive,
				GPUMemoryGB:   16.0,
			},
			{
				ID:            "model-b",
				ContainerName: "vllm-b",
				Port:          8000,
				HostPort:      8002,
				StartupMode:   models.StartupSleep,
				GPUMemoryGB:   24.0,
			},
		},
	}

	mockClient := vllm.NewMockClient()

	// Mock model-a as healthy+awake, model-b as sleeping
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-b" {
			return true, nil // model-b is sleeping
		}
		return false, nil // model-a is awake
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))

	// Wait for initial resync to complete
	s.WaitForInit()

	resp := s.GetModels()

	if len(resp.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(resp.Models))
	}

	if resp.ActiveModel != "model-a" {
		t.Errorf("expected active model 'model-a', got '%s'", resp.ActiveModel)
	}
}

func TestSwitchModel_Success(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{
				ID:            "model-a",
				ContainerName: "vllm-a",
				Port:          8000,
				HostPort:      8001,
				StartupMode:   models.StartupActive,
				GPUMemoryGB:   16.0,
			},
			{
				ID:            "model-b",
				ContainerName: "vllm-b",
				Port:          8000,
				HostPort:      8002,
				StartupMode:   models.StartupSleep,
				GPUMemoryGB:   24.0,
			},
		},
	}

	mockClient := vllm.NewMockClient()

	mockClient.HealthFunc = func(ctx context.Context, host string, port int) (bool, error) {
		return true, nil
	}

	// Model-b starts sleeping, model-a awake
	// After Sleep() is called on a model, it becomes sleeping
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-b" && port == 8000 {
			return true, nil // model-b starts sleeping
		}
		for _, call := range mockClient.SleepCalls {
			if call.Host == host && call.Port == port {
				return true, nil
			}
		}
		return false, nil
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))
	s.WaitForInit()

	ctx := context.Background()
	err := s.SwitchModel(ctx, "model-b")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify sleep was called on model-a
	if len(mockClient.SleepCalls) != 1 {
		t.Errorf("expected 1 sleep call, got %d", len(mockClient.SleepCalls))
	} else {
		call := mockClient.SleepCalls[0]
		if call.Host != "vllm-a" || call.Port != 8000 {
			t.Errorf("expected sleep on vllm-a:8000, got %s:%d", call.Host, call.Port)
		}
	}

	// Verify wake_up was called on model-b
	if len(mockClient.WakeUpCalls) != 1 {
		t.Errorf("expected 1 wake_up call, got %d", len(mockClient.WakeUpCalls))
	} else {
		call := mockClient.WakeUpCalls[0]
		if call.Host != "vllm-b" || call.Port != 8000 {
			t.Errorf("expected wake_up on vllm-b:8000, got %s:%d", call.Host, call.Port)
		}
	}

	// Verify active model changed
	if s.activeModel != "model-b" {
		t.Errorf("expected active model 'model-b', got '%s'", s.activeModel)
	}

	// Verify model statuses
	if s.models["model-a"].GetStatus() != models.StatusSleeping {
		t.Errorf("expected model-a to be sleeping, got %s", s.models["model-a"].GetStatus())
	}
	if s.models["model-b"].GetStatus() != models.StatusActive {
		t.Errorf("expected model-b to be active, got %s", s.models["model-b"].GetStatus())
	}
}

func TestSwitchModel_AlreadyActive(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive},
		},
	}

	mockClient := vllm.NewMockClient()
	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))
	s.WaitForInit()

	ctx := context.Background()
	err := s.SwitchModel(ctx, "model-a")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should not call sleep or wake_up
	if len(mockClient.SleepCalls) != 0 {
		t.Errorf("expected 0 sleep calls, got %d", len(mockClient.SleepCalls))
	}
	if len(mockClient.WakeUpCalls) != 0 {
		t.Errorf("expected 0 wake_up calls, got %d", len(mockClient.WakeUpCalls))
	}
}

func TestSwitchModel_ModelNotFound(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", StartupMode: models.StartupActive},
		},
	}

	mockClient := vllm.NewMockClient()
	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))
	s.WaitForInit()

	ctx := context.Background()
	err := s.SwitchModel(ctx, "nonexistent")

	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}

	if err.Error() != "model nonexistent not found" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSwitchModel_SleepFails(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive, GPUMemoryGB: 16.0},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8000, HostPort: 8002, StartupMode: models.StartupSleep, GPUMemoryGB: 24.0},
		},
	}

	mockClient := vllm.NewMockClient()
	mockClient.SleepFunc = func(ctx context.Context, host string, port int, level int) error {
		return errors.New("sleep failed")
	}
	// Ensure model-a is active, model-b sleeping
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-b" && port == 8000 {
			return true, nil
		}
		return false, nil
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))
	s.WaitForInit()

	ctx := context.Background()
	err := s.SwitchModel(ctx, "model-b")

	if err == nil {
		t.Fatal("expected error when sleep fails")
	}

	// Active model should remain model-a
	if s.activeModel != "model-a" {
		t.Errorf("expected active model to remain 'model-a', got '%s'", s.activeModel)
	}
}

func TestSwitchModel_WakeUpFails(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive, GPUMemoryGB: 16.0},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8000, HostPort: 8002, StartupMode: models.StartupSleep, GPUMemoryGB: 24.0},
		},
	}

	mockClient := vllm.NewMockClient()
	mockClient.WakeUpFunc = func(ctx context.Context, host string, port int) error {
		return errors.New("wake_up failed")
	}
	// Ensure model-a is active, model-b sleeping
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-b" && port == 8000 {
			return true, nil
		}
		for _, call := range mockClient.SleepCalls {
			if call.Host == host && call.Port == port {
				return true, nil
			}
		}
		return false, nil
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))
	s.WaitForInit()

	ctx := context.Background()
	err := s.SwitchModel(ctx, "model-b")

	if err == nil {
		t.Fatal("expected error when wake_up fails")
	}

	// Should have attempted to reactivate model-a
	if len(mockClient.WakeUpCalls) < 2 {
		t.Errorf("expected at least 2 wake_up calls (failed + rollback), got %d", len(mockClient.WakeUpCalls))
	}
}

func TestSwitchModel_HealthCheckFails(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive, GPUMemoryGB: 16.0},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8000, HostPort: 8002, StartupMode: models.StartupSleep, GPUMemoryGB: 24.0},
		},
	}

	mockClient := vllm.NewMockClient()
	mockClient.HealthFunc = func(ctx context.Context, host string, port int) (bool, error) {
		return false, errors.New("health check failed")
	}
	// Immediately confirm sleep to avoid retry delays
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-b" && port == 8000 {
			return true, nil
		}
		for _, call := range mockClient.SleepCalls {
			if call.Host == host && call.Port == port {
				return true, nil
			}
		}
		return false, nil
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))
	s.WaitForInit()

	ctx := context.Background()
	err := s.SwitchModel(ctx, "model-b")

	if err == nil {
		t.Fatal("expected error when health check fails")
	}

	// Should have retried health checks
	if len(mockClient.HealthCalls) < 2 {
		t.Errorf("expected at least 2 health calls, got %d", len(mockClient.HealthCalls))
	}
}

func TestDetermineSleepLevel(t *testing.T) {
	tests := []struct {
		name          string
		availableRAM  float64
		gpuMemory     float64
		expectedLevel int
	}{
		{
			name:          "sufficient RAM for level 1",
			availableRAM:  64.0,
			gpuMemory:     24.0,
			expectedLevel: 1,
		},
		{
			name:          "insufficient RAM, use level 2",
			availableRAM:  16.0,
			gpuMemory:     24.0,
			expectedLevel: 2,
		},
		{
			name:          "exactly sufficient RAM",
			availableRAM:  24.0,
			gpuMemory:     24.0,
			expectedLevel: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &models.Config{
				Models: []models.Model{
					{ID: "model-a", StartupMode: models.StartupActive, GPUMemoryGB: tt.gpuMemory},
				},
			}

			mockClient := vllm.NewMockClient()
			mockRAM := &system.MockRAMFetcher{AvailableRAMGB: tt.availableRAM}
			s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond), WithRAMFetcher(mockRAM))
			s.WaitForInit()

			level := s.determineSleepLevel(s.models["model-a"])

			if level != tt.expectedLevel {
				t.Errorf("expected sleep level %d, got %d", tt.expectedLevel, level)
			}
		})
	}
}

func TestResyncModels(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8000, HostPort: 8002, StartupMode: models.StartupSleep},
		},
	}

	mockClient := vllm.NewMockClient()

	// Simulate model-b is actually active, model-a is sleeping
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-a" {
			return true, nil
		}
		return false, nil
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))

	// Don't start background routines, call resync directly
	ctx := context.Background()
	err := s.resyncModels(ctx)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// After resync, model-b should be active
	if s.activeModel != "model-b" {
		t.Errorf("expected active model 'model-b' after resync, got '%s'", s.activeModel)
	}

	if s.models["model-a"].GetStatus() != models.StatusSleeping {
		t.Errorf("expected model-a to be sleeping after resync, got %s", s.models["model-a"].GetStatus())
	}

	if s.models["model-b"].GetStatus() != models.StatusActive {
		t.Errorf("expected model-b to be active after resync, got %s", s.models["model-b"].GetStatus())
	}
}

func TestResyncModels_WithErrors(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8000, HostPort: 8002, StartupMode: models.StartupSleep},
		},
	}

	mockClient := vllm.NewMockClient()

	// Simulate error querying model-a, model-b is active
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-a" {
			return false, errors.New("connection refused")
		}
		return false, nil
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))

	ctx := context.Background()
	err := s.resyncModels(ctx)

	// Should return an error but still update what it can
	if err == nil {
		t.Error("expected error when resync fails for a model")
	}

	// model-a should be marked as error
	if s.models["model-a"].GetStatus() != models.StatusError {
		t.Errorf("expected model-a to be error after failed resync, got %s", s.models["model-a"].GetStatus())
	}

	// model-b should still be updated correctly
	if s.models["model-b"].GetStatus() != models.StatusActive {
		t.Errorf("expected model-b to be active after resync, got %s", s.models["model-b"].GetStatus())
	}

	if s.activeModel != "model-b" {
		t.Errorf("expected active model 'model-b' after resync, got '%s'", s.activeModel)
	}
}

func TestNew_WithDisabledModel(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{
				ID:            "model-a",
				ContainerName: "vllm-a",
				Port:          8000,
				HostPort:      8001,
				StartupMode:   models.StartupActive,
				GPUMemoryGB:   16.0,
			},
			{
				ID:            "model-b",
				ContainerName: "vllm-b",
				Port:          8000,
				HostPort:      8002,
				StartupMode:   models.StartupDisabled,
				GPUMemoryGB:   24.0,
			},
		},
	}

	mockClient := vllm.NewMockClient()
	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))

	// Check disabled model has correct status
	if s.models["model-b"].GetStatus() != models.StatusDisabled {
		t.Errorf("expected model-b to be disabled, got %s", s.models["model-b"].GetStatus())
	}
}

func TestSwitchModel_DisabledModel(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8000, HostPort: 8002, StartupMode: models.StartupDisabled},
		},
	}

	mockClient := vllm.NewMockClient()
	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))
	s.WaitForInit()

	ctx := context.Background()
	err := s.SwitchModel(ctx, "model-b")

	if err == nil {
		t.Fatal("expected error when switching to disabled model")
	}

	if err.Error() != "model model-b is disabled and cannot be activated" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResyncModels_SkipsDisabled(t *testing.T) {
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, HostPort: 8001, StartupMode: models.StartupActive},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8000, HostPort: 8002, StartupMode: models.StartupDisabled},
		},
	}

	mockClient := vllm.NewMockClient()
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		// This should not be called for disabled models
		if host == "vllm-b" {
			t.Error("IsSleeping should not be called for disabled model")
		}
		return false, nil
	}

	s := NewWithClient(cfg, mockClient, WithMaxRetries(2), WithHealthCheckInterval(10*time.Millisecond))

	ctx := context.Background()
	err := s.resyncModels(ctx)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Disabled model should remain disabled
	if s.models["model-b"].GetStatus() != models.StatusDisabled {
		t.Errorf("expected model-b to remain disabled, got %s", s.models["model-b"].GetStatus())
	}
}
