package models

import (
	"encoding/json"
	"sync"
	"time"
)

// ModelStatus represents the current state of a model
type ModelStatus string

const (
	StatusActive    ModelStatus = "active"
	StatusSleeping  ModelStatus = "sleeping"
	StatusSwitching ModelStatus = "switching"
	StatusError     ModelStatus = "error"
)

// Model represents a vLLM model configuration and state
type Model struct {
	mu sync.Mutex // Protects mutable fields (status, lastActive)

	// Immutable config fields (set once, read-only after init)
	ID            string  `json:"id" yaml:"id"`
	Name          string  `json:"name" yaml:"name"`
	ContainerName string  `json:"container_name" yaml:"container_name"`
	Port          int     `json:"port" yaml:"port"`
	Default       bool    `json:"default" yaml:"default"`
	GPUMemoryGB   float64 `json:"gpu_memory_gb" yaml:"gpu_memory_gb"`

	// Mutable state fields (protected by mu)
	status     ModelStatus
	lastActive *time.Time
}

// GetStatus returns the current status (thread-safe)
func (m *Model) GetStatus() ModelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// SetStatus updates the status (thread-safe)
func (m *Model) SetStatus(status ModelStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
}

// GetLastActive returns the last active time (thread-safe)
func (m *Model) GetLastActive() *time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastActive == nil {
		return nil
	}
	t := *m.lastActive
	return &t
}

// SetLastActive updates the last active time (thread-safe)
func (m *Model) SetLastActive(t *time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t == nil {
		m.lastActive = nil
	} else {
		copy := *t
		m.lastActive = &copy
	}
}

// MarkActive sets status to active and updates last active time (thread-safe)
func (m *Model) MarkActive() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = StatusActive
	now := time.Now()
	m.lastActive = &now
}

// MarkSleeping sets status to sleeping (thread-safe)
func (m *Model) MarkSleeping() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = StatusSleeping
}

// MarkSwitching sets status to switching (thread-safe)
func (m *Model) MarkSwitching() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = StatusSwitching
}

// MarkError sets status to error (thread-safe)
func (m *Model) MarkError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = StatusError
}

// Snapshot returns a copy of the model with current state (thread-safe)
func (m *Model) Snapshot() Model {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastActiveCopy *time.Time
	if m.lastActive != nil {
		t := *m.lastActive
		lastActiveCopy = &t
	}

	return Model{
		ID:            m.ID,
		Name:          m.Name,
		ContainerName: m.ContainerName,
		Port:          m.Port,
		Default:       m.Default,
		GPUMemoryGB:   m.GPUMemoryGB,
		status:        m.status,
		lastActive:    lastActiveCopy,
		// mu is intentionally NOT copied - each snapshot gets zero value
	}
}

// MarshalJSON implements custom JSON marshaling (thread-safe)
func (m *Model) MarshalJSON() ([]byte, error) {
	snapshot := m.Snapshot()

	type ModelJSON struct {
		ID            string      `json:"id"`
		Name          string      `json:"name"`
		ContainerName string      `json:"container_name"`
		Port          int         `json:"port"`
		Default       bool        `json:"default"`
		GPUMemoryGB   float64     `json:"gpu_memory_gb"`
		Status        ModelStatus `json:"status"`
		LastActive    *time.Time  `json:"last_active,omitempty"`
	}

	j := ModelJSON{
		ID:            snapshot.ID,
		Name:          snapshot.Name,
		ContainerName: snapshot.ContainerName,
		Port:          snapshot.Port,
		Default:       snapshot.Default,
		GPUMemoryGB:   snapshot.GPUMemoryGB,
		Status:        snapshot.status,
		LastActive:    snapshot.lastActive,
	}

	return json.Marshal(j)
}

// Config represents the application configuration
type Config struct {
	Models    []Model         `yaml:"models"`
	Switching SwitchingConfig `yaml:"switching"`
}

// SwitchingConfig contains switching behavior settings
type SwitchingConfig struct {
	HealthCheckIntervalSeconds int     `yaml:"health_check_interval_seconds"`
	MaxRetries                 int     `yaml:"max_retries"`
	AvailableRAMGB             float64 `yaml:"available_ram_gb"`
}

// SwitchRequest is the request body for switching models
type SwitchRequest struct {
	ModelID string `json:"model_id" binding:"required"`
}

// ModelsResponse is the response for listing models
type ModelsResponse struct {
	Models      []Model `json:"models"`
	ActiveModel string  `json:"active_model"`
}

// HealthResponse is a simple health check response
type HealthResponse struct {
	Status string `json:"status"`
}
