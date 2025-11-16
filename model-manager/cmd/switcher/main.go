package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/zheng/homeGPT/internal/config"
	"github.com/zheng/homeGPT/internal/handlers"
	"github.com/zheng/homeGPT/internal/switcher"
)

func main() {
	// Load configuration
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/app/config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded configuration with %d models", len(cfg.Models))

	// Initialize switcher
	sw := switcher.New(cfg)

	// Initialize handlers
	h := handlers.New(sw)

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// CORS middleware for internal service
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Routes
	r.GET("/health", h.Health)
	r.GET("/models", h.GetModels)
	r.POST("/switch", h.SwitchModel)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	log.Printf("Starting model manager service on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
