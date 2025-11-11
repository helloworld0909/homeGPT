package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zheng/homeGPT/internal/switcher"
	"github.com/zheng/homeGPT/internal/vllm"
	"github.com/zheng/homeGPT/pkg/models"
)

func setupTestHandler() (*Handler, *vllm.MockClient) {
	gin.SetMode(gin.TestMode)

	cfg := &models.Config{
		Models: []models.Model{
			{
				ID:            "model-a",
				Name:          "Model A",
				ContainerName: "vllm-a",
				Port:          8000,
				Default:       true,
			},
			{
				ID:            "model-b",
				Name:          "Model B",
				ContainerName: "vllm-b",
				Port:          8001,
				Default:       false,
			},
		},
		Switching: models.SwitchingConfig{
			HealthCheckIntervalSeconds: 1,
			MaxRetries:                 3,
			AvailableRAMGB:             64.0,
		},
	}

	mockClient := vllm.NewMockClient()

	// Track which models have been put to sleep
	sleptModels := make(map[string]bool)

	// Mock default behavior: all models healthy
	mockClient.HealthFunc = func(ctx context.Context, host string, port int) (bool, error) {
		return true, nil
	}

	// Mock sleeping state: model-b starts sleeping, others awake
	// After Sleep() is called, that model becomes sleeping
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if host == "vllm-b" && port == 8001 {
			return true, nil // model-b starts sleeping
		}
		// Check if this model was put to sleep
		key := host + ":" + string(rune(port))
		if sleptModels[key] {
			return true, nil
		}
		return false, nil
	}

	// Track sleep calls
	mockClient.SleepFunc = func(ctx context.Context, host string, port int, level int) error {
		key := host + ":" + string(rune(port))
		sleptModels[key] = true
		return nil
	}

	s := switcher.NewWithClient(cfg, mockClient)
	s.WaitForInit()

	h := New(s)
	return h, mockClient
}

func TestHealth(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/health", nil)
	c.Request = req

	h.Health(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp models.HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", resp.Status)
	}
}

func TestGetModels(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/models", nil)
	c.Request = req

	h.GetModels(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp models.ModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(resp.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(resp.Models))
	}

	if resp.ActiveModel != "model-a" {
		t.Errorf("expected active model 'model-a', got '%s'", resp.ActiveModel)
	}
}

func TestSwitchModel_Success(t *testing.T) {
	h, mockClient := setupTestHandler()

	reqBody := models.SwitchRequest{
		ModelID: "model-b",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/switch", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.SwitchModel(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "success" {
		t.Errorf("expected status 'success', got '%s'", resp["status"])
	}

	if resp["active_model"] != "model-b" {
		t.Errorf("expected active_model 'model-b', got '%s'", resp["active_model"])
	}

	// Verify vLLM calls were made
	if len(mockClient.SleepCalls) != 1 {
		t.Errorf("expected 1 sleep call, got %d", len(mockClient.SleepCalls))
	}

	if len(mockClient.WakeUpCalls) != 1 {
		t.Errorf("expected 1 wake_up call, got %d", len(mockClient.WakeUpCalls))
	}
}

func TestSwitchModel_InvalidJSON(t *testing.T) {
	h, _ := setupTestHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/switch", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.SwitchModel(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if _, ok := resp["error"]; !ok {
		t.Error("expected error field in response")
	}
}

func TestSwitchModel_MissingModelID(t *testing.T) {
	h, _ := setupTestHandler()

	reqBody := map[string]string{} // Empty body, missing model_id
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/switch", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.SwitchModel(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestSwitchModel_NonexistentModel(t *testing.T) {
	h, _ := setupTestHandler()

	reqBody := models.SwitchRequest{
		ModelID: "nonexistent",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/switch", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.SwitchModel(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if _, ok := resp["error"]; !ok {
		t.Error("expected error field in response")
	}
}

func TestSwitchModel_SwitchFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, Default: true},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8001, Default: false},
		},
		Switching: models.SwitchingConfig{
			HealthCheckIntervalSeconds: 1,
			MaxRetries:                 3,
			AvailableRAMGB:             64.0,
		},
	}

	mockClient := vllm.NewMockClient()
	mockClient.SleepFunc = func(ctx context.Context, host string, port int, level int) error {
		return errors.New("sleep failed")
	}

	s := switcher.NewWithClient(cfg, mockClient)
	s.WaitForInit()

	h := New(s)

	reqBody := models.SwitchRequest{
		ModelID: "model-b",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/switch", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.SwitchModel(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if _, ok := resp["error"]; !ok {
		t.Error("expected error field in response")
	}
}

func TestSwitchModel_AlreadyActive(t *testing.T) {
	h, mockClient := setupTestHandler()

	reqBody := models.SwitchRequest{
		ModelID: "model-a", // Already active
	}
	bodyBytes, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("POST", "/switch", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.SwitchModel(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Should not make any vLLM calls
	if len(mockClient.SleepCalls) != 0 {
		t.Errorf("expected 0 sleep calls, got %d", len(mockClient.SleepCalls))
	}

	if len(mockClient.WakeUpCalls) != 0 {
		t.Errorf("expected 0 wake_up calls, got %d", len(mockClient.WakeUpCalls))
	}
}
