package handlers

import (
	"log"
	"net/http"

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
