package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zheng/homeGPT/internal/switcher"
	"github.com/zheng/homeGPT/pkg/models"
)

// Handler handles HTTP requests for model switching
type Handler struct {
	switcher *switcher.Switcher
}

// New creates a new HTTP handler
func New(s *switcher.Switcher) *Handler {
	return &Handler{
		switcher: s,
	}
}

// Health returns the service health status
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, models.HealthResponse{
		Status: "healthy",
	})
}

// GetModels returns the list of all models and their status
func (h *Handler) GetModels(c *gin.Context) {
	resp := h.switcher.GetModels()
	c.JSON(http.StatusOK, resp)
}

// SwitchModel handles model switching requests
func (h *Handler) SwitchModel(c *gin.Context) {
	var req models.SwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("Received switch request to model: %s", req.ModelID)

	if err := h.switcher.SwitchModel(c.Request.Context(), req.ModelID); err != nil {
		log.Printf("Switch failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"active_model": req.ModelID,
	})
}

// ProxyHandler handles wildcard requests to /v1/*
// It inspects the request for a model ID, validates it against the active model, and forwards the request
func (h *Handler) ProxyHandler(c *gin.Context) {
	// Special case: intercept GET /v1/models to return full model list
	if c.Request.URL.Path == "/v1/models" && c.Request.Method == http.MethodGet {
		h.ListOpenAIModels(c)
		return
	}

	// 1. Inspect request to validate model
	// Only check body for endpoints that require a model parameter
	// (chat/completions, completions, embeddings, etc. - NOT files, uploads, etc.)
	if requiresModelValidation(c.Request.URL.Path) {
		if err := h.validateRequestModel(c); err != nil {
			log.Printf("Proxy validation failed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// 2. Get the current active model to forward to
	resp := h.switcher.GetModels()
	var activeModel *models.Model
	for i := range resp.Models {
		if resp.Models[i].ID == resp.ActiveModel {
			activeModel = &resp.Models[i]
			break
		}
	}

	if activeModel == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no active model"})
		return
	}

	// 3. Create Reverse Proxy
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d", activeModel.ContainerName, activeModel.Port))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid active model url"})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Update the director to preserve the original path
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// The targetURL path is empty, so JoinPath logic in NewSingleHostReverseProxy might be tricky
		// But we want to keep the original request path (e.g. /v1/chat/completions)
		// NewSingleHostReverseProxy sets req.URL.Scheme and req.URL.Host
		// It also joins targetURL.Path with req.URL.Path.
		// Since targetURL.Path is empty, it should just be req.URL.Path.
		// However, we need to ensure the Host header is set correctly for some backends
		req.Host = targetURL.Host
	}

	// 4. Serve
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) validateRequestModel(c *gin.Context) error {
	// Read body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}

	// Restore body for the proxy
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Parse model field
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		// If we can't parse JSON, just ignore (might be some other type of request)
		return nil
	}

	if req.Model == "" {
		return nil
	}

	// Check if the requested model matches the active model
	resp := h.switcher.GetModels()
	if resp.ActiveModel == req.Model {
		return nil
	}

	return fmt.Errorf("requested model '%s' is not active (current: '%s'). Please switch model explicitly via /switch endpoint", req.Model, resp.ActiveModel)
}

// requiresModelValidation determines if a given path requires model validation
// Returns true for endpoints that need a model parameter (chat, completions, embeddings, etc.)
// Returns false for endpoints that don't use models (files, uploads, etc.)
func requiresModelValidation(path string) bool {
	// Endpoints that require model validation
	modelRequiredPaths := []string{
		"/v1/chat/completions",
		"/v1/completions",
		"/v1/embeddings",
		"/v1/audio/transcriptions",
		"/v1/audio/translations",
		"/v1/images/generations",
		"/v1/images/edits",
		"/v1/images/variations",
		"/v1/moderations",
	}

	for _, p := range modelRequiredPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}

	return false
}

// OpenAI Response Structs
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type OpenAIListModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// ListOpenAIModels handles GET /v1/models
// It returns the list of all configured models in OpenAI format
func (h *Handler) ListOpenAIModels(c *gin.Context) {
	resp := h.switcher.GetModels()
	
	openAIModels := make([]OpenAIModel, 0, len(resp.Models))
	for i := range resp.Models {
		m := &resp.Models[i]
		// Skip disabled models if desired, but config usually implies availability
		if m.StartupMode == models.StartupDisabled {
			continue
		}

		openAIModels = append(openAIModels, OpenAIModel{
			ID:      m.ID,
			Object:  "model",
			Created: time.Now().Unix(), // Dummy timestamp or use m.LastActive if available
			OwnedBy: "homeGPT",
		})
	}

	c.JSON(http.StatusOK, OpenAIListModelsResponse{
		Object: "list",
		Data:   openAIModels,
	})
}
