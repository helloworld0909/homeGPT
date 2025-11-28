package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

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
				StartupMode:   models.StartupActive,
			},
			{
				ID:            "model-b",
				Name:          "Model B",
				ContainerName: "vllm-b",
				Port:          8001,
				StartupMode:   models.StartupSleep,
			},
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

func TestHandler_Health(t *testing.T) {
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

func TestHandler_GetModels(t *testing.T) {
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

func TestHandler_SwitchModel_Success(t *testing.T) {
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

func TestHandler_SwitchModel_InvalidJSON(t *testing.T) {
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

func TestHandler_SwitchModel_MissingModelID(t *testing.T) {
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

func TestHandler_SwitchModel_NonexistentModel(t *testing.T) {
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

func TestHandler_SwitchModel_SwitchFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &models.Config{
		Models: []models.Model{
			{ID: "model-a", ContainerName: "vllm-a", Port: 8000, StartupMode: models.StartupActive},
			{ID: "model-b", ContainerName: "vllm-b", Port: 8001, StartupMode: models.StartupSleep},
		},
	}

	mockClient := vllm.NewMockClient()
	// Return correct sleeping states
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		// model-b starts sleeping, model-a starts active
		if host == "vllm-b" {
			return true, nil
		}
		// After sleep is called on model-a, it becomes sleeping
		for _, call := range mockClient.SleepCalls {
			if call.Host == host && call.Port == port {
				return true, nil
			}
		}
		return false, nil
	}
	// Make wake_up fail for model-b
	mockClient.WakeUpFunc = func(ctx context.Context, host string, port int) error {
		if host == "vllm-b" {
			return errors.New("wake up failed")
		}
		return nil
	}

	s := switcher.NewWithClient(cfg, mockClient, switcher.WithMaxRetries(2), switcher.WithHealthCheckInterval(10*time.Millisecond))
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

func TestHandler_SwitchModel_AlreadyActive(t *testing.T) {
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

type CloseNotifyingRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

func NewCloseNotifyingRecorder() *CloseNotifyingRecorder {
	return &CloseNotifyingRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closed:           make(chan bool),
	}
}

func (r *CloseNotifyingRecorder) CloseNotify() <-chan bool {
	return r.closed
}

func setupProxyTest() (*Handler, *httptest.Server) {
	gin.SetMode(gin.TestMode)

	// Mock vLLM backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend response"))
	}))

	// Parse backend URL
	u, _ := url.Parse(backend.URL)
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())

	cfg := &models.Config{
		Models: []models.Model{
			{
				ID:            "model-a",
				Name:          "Model A",
				ContainerName: host,
				Port:          port,
				StartupMode:   models.StartupActive,
			},
			{
				ID:            "model-b",
				Name:          "Model B",
				ContainerName: "vllm-b",
				Port:          8001,
				StartupMode:   models.StartupSleep,
			},
		},
	}

	mockClient := vllm.NewMockClient()
	mockClient.HealthFunc = func(ctx context.Context, host string, port int) (bool, error) {
		return true, nil
	}
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) {
		if port == 8001 {
			return true, nil
		}
		return false, nil
	}

	s := switcher.NewWithClient(cfg, mockClient)
	s.WaitForInit()

	h := New(s)
	return h, backend
}

func TestHandler_Proxy_ValidRequest(t *testing.T) {
	h, backend := setupProxyTest()
	defer backend.Close()

	w := NewCloseNotifyingRecorder()
	c, _ := gin.CreateTestContext(w)

	reqBody := map[string]string{"model": "model-a"}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.ProxyHandler(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "backend response" {
		t.Errorf("expected 'backend response', got '%s'", w.Body.String())
	}
}

func TestHandler_Proxy_InactiveModel(t *testing.T) {
	h, backend := setupProxyTest()
	defer backend.Close()

	w := NewCloseNotifyingRecorder()
	c, _ := gin.CreateTestContext(w)

	reqBody := map[string]string{"model": "model-b"}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.ProxyHandler(c)

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

func TestHandler_Proxy_MissingModelField(t *testing.T) {
	h, backend := setupProxyTest()
	defer backend.Close()

	w := NewCloseNotifyingRecorder()
	c, _ := gin.CreateTestContext(w)

	reqBody := map[string]string{"foo": "bar"} // No model field
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.ProxyHandler(c)

	// Should forward to active model (model-a)
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "backend response" {
		t.Errorf("expected 'backend response', got '%s'", w.Body.String())
	}
}

func TestHandler_Proxy_GetRequest(t *testing.T) {
	h, backend := setupProxyTest()
	defer backend.Close()

	w := NewCloseNotifyingRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	c.Request = req

	h.ProxyHandler(c)

	// Should forward to active model
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Expect the handler to return the OpenAI-style models list
	var listResp OpenAIListModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to unmarshal ListOpenAIModels response: %v", err)
	}

	if listResp.Object != "list" {
		t.Errorf("expected object 'list', got '%s'", listResp.Object)
	}

	if len(listResp.Data) != 2 {
		t.Errorf("expected 2 models in list, got %d", len(listResp.Data))
	}

	// Verify IDs are present
	ids := map[string]bool{}
	for _, m := range listResp.Data {
		ids[m.ID] = true
	}
	if !ids["model-a"] || !ids["model-b"] {
		t.Errorf("expected model-a and model-b in list, got %v", ids)
	}
}

func TestHandler_Proxy_NonJSONBody_Forwards(t *testing.T) {
	h, backend := setupProxyTest()
	defer backend.Close()

	w := NewCloseNotifyingRecorder()
	c, _ := gin.CreateTestContext(w)

	// Non-JSON body should not cause validation to block proxying
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString("not a json body"))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.ProxyHandler(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "backend response" {
		t.Errorf("expected 'backend response', got '%s'", w.Body.String())
	}
}

func TestHandler_Proxy_NoValidationPath_Forwards(t *testing.T) {
	h, backend := setupProxyTest()
	defer backend.Close()

	w := NewCloseNotifyingRecorder()
	c, _ := gin.CreateTestContext(w)

	// /v1/files should not require model validation
	req := httptest.NewRequest("POST", "/v1/files", bytes.NewBufferString(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.ProxyHandler(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "backend response" {
		t.Errorf("expected 'backend response', got '%s'", w.Body.String())
	}
}

func TestHandler_Proxy_NoActiveModel_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create config where all models are disabled so activeModel == ""
	cfg := &models.Config{
		Models: []models.Model{
			{ID: "m1", ContainerName: "v1", Port: 8000, StartupMode: models.StartupDisabled},
			{ID: "m2", ContainerName: "v2", Port: 8001, StartupMode: models.StartupDisabled},
		},
	}

	mockClient := vllm.NewMockClient()
	// vllm client shouldn't be called, but implement safe defaults
	mockClient.HealthFunc = func(ctx context.Context, host string, port int) (bool, error) { return true, nil }
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) { return false, nil }

	s := switcher.NewWithClient(cfg, mockClient)
	s.WaitForInit()

	h := New(s)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Send a request without a `model` field so validation does not short-circuit
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	h.ProxyHandler(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandler_RequiresModelValidation_Prefixes(t *testing.T) {
	// Paths that should require validation
	truePaths := []string{
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
		"/v1/audio/transcriptions",
	}

	for _, p := range truePaths {
		if !requiresModelValidation(p) {
			t.Errorf("expected requiresModelValidation(%s) == true", p)
		}
	}

	// Paths that should NOT require validation
	falsePaths := []string{
		"/v1/files",
		"/v1/uploads",
		"/v1/some/other",
	}
	for _, p := range falsePaths {
		if requiresModelValidation(p) {
			t.Errorf("expected requiresModelValidation(%s) == false", p)
		}
	}
}

func TestHandler_ListOpenAIModels_SkipsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &models.Config{
		Models: []models.Model{
			{ID: "a", ContainerName: "a", Port: 8000, StartupMode: models.StartupActive},
			{ID: "b", ContainerName: "b", Port: 8001, StartupMode: models.StartupDisabled},
		},
	}

	mockClient := vllm.NewMockClient()
	mockClient.HealthFunc = func(ctx context.Context, host string, port int) (bool, error) { return true, nil }
	mockClient.IsSleepingFunc = func(ctx context.Context, host string, port int) (bool, error) { return false, nil }

	s := switcher.NewWithClient(cfg, mockClient)
	s.WaitForInit()

	h := New(s)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequest("GET", "/v1/models", nil)
	c.Request = req

	h.ListOpenAIModels(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp OpenAIListModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	for _, m := range resp.Data {
		if m.ID == "b" {
			t.Errorf("expected disabled model 'b' to be skipped in ListOpenAIModels")
		}
	}
}
