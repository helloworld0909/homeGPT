package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	content := `
models:
  - id: model-a
    name: "Model A"
    container_name: "vllm-a"
    port: 8000
    host_port: 8001
    startup_mode: active
    gpu_memory_gb: 16.0
  - id: model-b
    name: "Model B"
    container_name: "vllm-b"
    port: 8000
    host_port: 8002
    startup_mode: sleep
    gpu_memory_gb: 24.0
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(cfg.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(cfg.Models))
	}

	if cfg.Models[0].ID != "model-a" {
		t.Errorf("expected first model id 'model-a', got '%s'", cfg.Models[0].ID)
	}

	if cfg.Models[0].StartupMode != "active" {
		t.Error("expected first model to have startup_mode='active'")
	}

	if cfg.Models[0].HostPort != 8001 {
		t.Errorf("expected first model host_port 8001, got %d", cfg.Models[0].HostPort)
	}

	if cfg.Models[0].StartupMode != "active" {
		t.Errorf("expected first model startup_mode 'active', got '%s'", cfg.Models[0].StartupMode)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")

	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	content := `
models:
  - id: model-a
    invalid yaml here {{
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_NoModels(t *testing.T) {
	content := `
models: []
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error for empty models list")
	}

	if err.Error() != "no models defined in config" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_NoDefaultModel(t *testing.T) {
	content := `
models:
  - id: model-a
    name: "Model A"
    container_name: "vllm-a"
    port: 8000
    host_port: 8001
    startup_mode: sleep
  - id: model-b
    name: "Model B"
    container_name: "vllm-b"
    port: 8000
    host_port: 8002
    startup_mode: sleep
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error when no model has startup_mode='active'")
	}

	if err.Error() != "exactly one model must have startup_mode='active', found 0" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_MultipleDefaultModels(t *testing.T) {
	content := `
models:
  - id: model-a
    name: "Model A"
    container_name: "vllm-a"
    port: 8000
    host_port: 8001
    startup_mode: active
  - id: model-b
    name: "Model B"
    container_name: "vllm-b"
    port: 8000
    host_port: 8002
    startup_mode: active
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error when multiple models have startup_mode='active'")
	}

	if err.Error() != "exactly one model must have startup_mode='active', found 2" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_MinimalConfig(t *testing.T) {
	content := `
models:
  - id: model-a
    container_name: "vllm-a"
    port: 8000
    host_port: 8001
    startup_mode: active
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("expected no error with minimal config, got %v", err)
	}

	if len(cfg.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(cfg.Models))
	}

	if cfg.Models[0].StartupMode != "active" {
		t.Errorf("expected startup_mode 'active', got '%s'", cfg.Models[0].StartupMode)
	}
}

func TestLoad_MissingStartupMode(t *testing.T) {
	content := `
models:
  - id: model-a
    container_name: "vllm-a"
    port: 8000
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error when startup_mode is missing")
	}
}

func TestLoad_StartupModeDisabled(t *testing.T) {
	content := `
models:
  - id: model-a
    container_name: "vllm-a"
    port: 8000
    host_port: 8001
    startup_mode: active
  - id: model-b
    container_name: "vllm-b"
    port: 8000
    host_port: 8002
    startup_mode: disabled
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Models[1].StartupMode != "disabled" {
		t.Errorf("expected startup_mode 'disabled' for model-b, got '%s'", cfg.Models[1].StartupMode)
	}
}
