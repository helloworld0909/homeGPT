package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestModel_StatusSetGet(t *testing.T) {
	m := &Model{ID: "test"}

	m.SetStatus(StatusActive)
	if s := m.GetStatus(); s != StatusActive {
		t.Fatalf("expected status %s, got %s", StatusActive, s)
	}

	m.MarkSleeping()
	if s := m.GetStatus(); s != StatusSleeping {
		t.Fatalf("expected status %s, got %s", StatusSleeping, s)
	}

	m.MarkSwitching()
	if s := m.GetStatus(); s != StatusSwitching {
		t.Fatalf("expected status %s, got %s", StatusSwitching, s)
	}

	m.MarkError()
	if s := m.GetStatus(); s != StatusError {
		t.Fatalf("expected status %s, got %s", StatusError, s)
	}

	m.MarkDisabled()
	if s := m.GetStatus(); s != StatusDisabled {
		t.Fatalf("expected status %s, got %s", StatusDisabled, s)
	}
}

func TestModel_LastActive_SetGetAndSnapshot(t *testing.T) {
	m := &Model{ID: "t2"}

	// Initially nil
	if t1 := m.GetLastActive(); t1 != nil {
		t.Fatalf("expected nil lastActive, got %v", t1)
	}

	now := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	m.SetLastActive(&now)

	got := m.GetLastActive()
	if got == nil || !got.Equal(now) {
		t.Fatalf("expected lastActive %v, got %v", now, got)
	}

	// Snapshot should copy the value (no shared pointer)
	snap := m.Snapshot()
	if snap.lastActive == nil || !snap.lastActive.Equal(now) {
		t.Fatalf("snapshot missing lastActive: %v", snap.lastActive)
	}

	// Mutate original's lastActive and ensure snapshot unaffected
	later := now.Add(time.Hour)
	m.SetLastActive(&later)
	if snap.lastActive.Equal(later) {
		t.Fatalf("snapshot should not reflect changes to original lastActive")
	}
}

func TestModel_MarkActive_SetsLastActiveAndStatus(t *testing.T) {
	m := &Model{ID: "t3"}

	m.MarkActive()
	if s := m.GetStatus(); s != StatusActive {
		t.Fatalf("expected status active after MarkActive, got %s", s)
	}

	if la := m.GetLastActive(); la == nil {
		t.Fatalf("expected lastActive to be set after MarkActive")
	}
}

func TestModel_MarshalJSON_IncludesFields(t *testing.T) {
	m := &Model{
		ID:            "mjson",
		Name:          "MyModel",
		ContainerName: "cont",
		Port:          8000,
		HostPort:      9000,
		GPUMemoryGB:   12.5,
		StartupMode:   StartupActive,
	}

	m.MarkActive()

	b, err := m.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal result failed: %v", err)
	}

	if out["id"] != "mjson" {
		t.Fatalf("expected id=mjson in json, got %v", out["id"])
	}

	if out["status"] != string(StatusActive) {
		t.Fatalf("expected status active in json, got %v", out["status"])
	}
}
