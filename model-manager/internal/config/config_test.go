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
    default: true
    gpu_memory_gb: 16.0
  - id: model-b
    name: "Model B"
    container_name: "vllm-b"
    port: 8001
    default: false
    gpu_memory_gb: 24.0

switching:
  health_check_interval_seconds: 2
  max_retries: 5
  available_ram_gb: 64.0
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

	if !cfg.Models[0].Default {
		t.Error("expected first model to be default")
	}

	if cfg.Switching.MaxRetries != 5 {
		t.Errorf("expected max_retries 5, got %d", cfg.Switching.MaxRetries)
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
switching:
  health_check_interval_seconds: 2
  max_retries: 5
  available_ram_gb: 64.0
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
    default: false
  - id: model-b
    name: "Model B"
    container_name: "vllm-b"
    port: 8001
    default: false
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error when no model is default")
	}

	if err.Error() != "exactly one model must be set as default, found 0" {
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
    default: true
  - id: model-b
    name: "Model B"
    container_name: "vllm-b"
    port: 8001
    default: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)

	if err == nil {
		t.Fatal("expected error when multiple models are default")
	}

	if err.Error() != "exactly one model must be set as default, found 2" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoad_MinimalConfig(t *testing.T) {
	content := `
models:
  - id: model-a
    container_name: "vllm-a"
    port: 8000
    default: true
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
}
