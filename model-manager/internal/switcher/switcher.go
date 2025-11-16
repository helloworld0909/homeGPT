package switcher

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/zheng/homeGPT/internal/system"
	"github.com/zheng/homeGPT/internal/utils"
	"github.com/zheng/homeGPT/internal/vllm"
	"github.com/zheng/homeGPT/pkg/models"
)

// Switcher manages model switching operations
type Switcher struct {
	config              *models.Config
	vllmClient          vllm.VLLMClient
	ramFetcher          system.RAMFetcher
	models              map[string]*models.Model
	activeModel         string
	healthCheckInterval time.Duration
	maxRetries          int
	mapMu               sync.RWMutex   // Protects models map and activeModel string only
	switchLock          sync.Mutex     // Ensures only one switch operation at a time
	initSync            sync.WaitGroup // Tracks initial resync completion
}

const (
	defaultHealthCheckInterval = 2 * time.Second
	defaultMaxRetries          = 450 // 15 minutes max startup time (450 * 2s = 900s)
)

// Option is a function that configures the Switcher
type Option func(*Switcher)

// WithHealthCheckInterval sets a custom health check interval
func WithHealthCheckInterval(interval time.Duration) Option {
	return func(s *Switcher) {
		s.healthCheckInterval = interval
	}
}

// WithMaxRetries sets a custom max retries count
func WithMaxRetries(retries int) Option {
	return func(s *Switcher) {
		s.maxRetries = retries
	}
}

// WithRAMFetcher sets a custom RAM fetcher for testing
func WithRAMFetcher(fetcher system.RAMFetcher) Option {
	return func(s *Switcher) {
		s.ramFetcher = fetcher
	}
}

// New creates a new model switcher
func New(cfg *models.Config, opts ...Option) *Switcher {
	return NewWithClient(cfg, vllm.NewClient(), opts...)
}

// NewWithClient creates a new model switcher with a custom vLLM client (for testing)
func NewWithClient(cfg *models.Config, client vllm.VLLMClient, opts ...Option) *Switcher {
	s := &Switcher{
		config:              cfg,
		vllmClient:          client,
		ramFetcher:          system.NewRAMFetcher(),
		models:              make(map[string]*models.Model),
		healthCheckInterval: defaultHealthCheckInterval,
		maxRetries:          defaultMaxRetries,
	}

	// Apply options
	for _, opt := range opts {
		opt(s)
	}

	// Initialize models based on startup_mode
	for i := range cfg.Models {
		model := &cfg.Models[i]

		switch model.StartupMode {
		case models.StartupDisabled:
			model.MarkDisabled()
		case models.StartupSleep:
			model.MarkSleeping()
		case models.StartupActive:
			model.MarkActive()
			s.activeModel = model.ID
		default:
			log.Fatalf("Invalid startup_mode '%s' for model %s. Must be 'disabled', 'sleep', or 'active'",
				model.StartupMode, model.ID)
		}

		s.models[model.ID] = model
	}

	// Perform an initial resync with the vLLM servers to ensure in-memory
	// state matches actual server state (containers may have restarted).
	s.initSync.Add(1)
	go func() {
		defer s.initSync.Done()

		cfg := utils.RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1600 * time.Millisecond,
			Multiplier:   2.0,
		}

		ctx := context.Background()
		err := utils.RetryWithBackoff(ctx, cfg, func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return s.resyncModels(ctx)
		})

		if err == nil {
			log.Printf("initial resync completed successfully")
		} else {
			log.Printf("initial resync failed after %d attempts: %v", cfg.MaxAttempts, err)
		}
	}()

	// Start a background periodic resync to keep state accurate.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := s.resyncModels(ctx); err != nil {
				log.Printf("periodic resync warning: %v", err)
			}
			cancel()
		}
	}()

	return s
}

// resyncModels queries each configured vLLM endpoint and updates the in-memory
// model statuses to reflect the actual server state. This helps recover from
// container restarts or out-of-band changes.
func (s *Switcher) resyncModels(ctx context.Context) error {
	s.mapMu.RLock()
	modelsCopy := make(map[string]*models.Model, len(s.models))
	for k, v := range s.models {
		modelsCopy[k] = v
	}
	s.mapMu.RUnlock()

	var lastActive string
	var anyErr error

	for id, m := range modelsCopy {
		// Skip disabled models
		if m.StartupMode == models.StartupDisabled {
			continue
		}

		// Query the vLLM server for sleep state
		sleeping, err := s.vllmClient.IsSleeping(ctx, m.ContainerName, m.Port)
		if err != nil {
			// mark as error but continue
			m.MarkError()
			anyErr = err
			log.Printf("resync: failed to query %s (%s:%d): %v", id, m.ContainerName, m.Port, err)
			continue
		}

		if sleeping {
			m.MarkSleeping()
		} else {
			m.MarkActive()
			// record the last active model (if multiple awake, first wins)
			if lastActive == "" {
				lastActive = id
			}
		}
	}

	s.mapMu.Lock()
	if lastActive != "" {
		s.activeModel = lastActive
	} else {
		// no model reported active; clear activeModel
		s.activeModel = ""
	}
	s.mapMu.Unlock()

	return anyErr
}

// GetModels returns current state of all models
func (s *Switcher) GetModels() models.ModelsResponse {
	s.mapMu.RLock()
	defer s.mapMu.RUnlock()

	modelList := make([]models.Model, 0, len(s.models))
	for _, m := range s.models {
		modelList = append(modelList, m.Snapshot())
	}

	return models.ModelsResponse{
		Models:      modelList,
		ActiveModel: s.activeModel,
	}
}

// WaitForInit waits for the initial resync to complete
func (s *Switcher) WaitForInit() {
	s.initSync.Wait()
}

// SwitchModel switches from the current active model to the target model
func (s *Switcher) SwitchModel(ctx context.Context, targetModelID string) error {
	// Acquire switch lock to prevent concurrent switches
	s.switchLock.Lock()
	defer s.switchLock.Unlock()

	s.mapMu.RLock()
	targetModel, exists := s.models[targetModelID]
	currentActive := s.activeModel
	s.mapMu.RUnlock()

	if !exists {
		return fmt.Errorf("model %s not found", targetModelID)
	}

	// Check if target model is disabled
	if targetModel.StartupMode == models.StartupDisabled {
		return fmt.Errorf("model %s is disabled and cannot be activated", targetModelID)
	}

	if targetModelID == currentActive {
		return nil // Already active
	}

	log.Printf("Starting switch from %s to %s", currentActive, targetModelID)

	// Step 1: Put current model to sleep
	if currentActive != "" {
		if err := s.sleepModel(ctx, currentActive); err != nil {
			return fmt.Errorf("failed to sleep current model: %w", err)
		}
	}

	// Step 2: Wake up target model
	if err := s.activateModel(ctx, targetModelID); err != nil {
		// Try to reactivate previous model
		if currentActive != "" {
			log.Printf("Failed to activate %s, attempting to reactivate %s", targetModelID, currentActive)
			s.activateModel(ctx, currentActive)
		}
		return fmt.Errorf("failed to activate target model: %w", err)
	}

	// Update active model
	s.mapMu.Lock()
	s.activeModel = targetModelID
	s.mapMu.Unlock()

	log.Printf("Successfully switched to %s", targetModelID)
	return nil
}

// sleepModel puts a model into sleep mode
func (s *Switcher) sleepModel(ctx context.Context, modelID string) error {
	s.mapMu.RLock()
	model := s.models[modelID]
	s.mapMu.RUnlock()

	model.MarkSwitching()

	log.Printf("Putting model %s to sleep", modelID)

	// Determine sleep level based on available RAM
	sleepLevel := s.determineSleepLevel(model)
	log.Printf("Using sleep level %d for model %s", sleepLevel, modelID)

	// Call vLLM sleep endpoint
	if err := s.vllmClient.Sleep(ctx, model.ContainerName, model.Port, sleepLevel); err != nil {
		model.MarkError()
		return fmt.Errorf("failed to sleep model: %w", err)
	}

	// Poll to confirm sleep state with exponential backoff
	// Use more attempts for sleep confirmation as it can take longer
	cfg := utils.RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     3200 * time.Millisecond,
		Multiplier:   2.0,
	}
	err := utils.PollUntil(ctx, cfg, func() (bool, error) {
		return s.vllmClient.IsSleeping(ctx, model.ContainerName, model.Port)
	})

	if err != nil {
		log.Printf("Warning: Could not confirm sleep state for %s: %v (assuming success)", modelID, err)
	}

	model.MarkSleeping()
	log.Printf("Model %s is now sleeping", modelID)
	return nil
}

// activateModel wakes up a model from sleep mode
func (s *Switcher) activateModel(ctx context.Context, modelID string) error {
	s.mapMu.RLock()
	model := s.models[modelID]
	s.mapMu.RUnlock()

	model.MarkSwitching()

	log.Printf("Waking up model %s", modelID)

	// Call vLLM wake_up endpoint
	if err := s.vllmClient.WakeUp(ctx, model.ContainerName, model.Port); err != nil {
		model.MarkError()
		return fmt.Errorf("failed to wake up model: %w", err)
	}

	// Wait for model to be healthy
	maxRetries := s.maxRetries
	interval := s.healthCheckInterval

	for i := 0; i < maxRetries; i++ {
		log.Printf("Health check %d/%d for model %s", i+1, maxRetries, modelID)

		healthy, err := s.vllmClient.Health(ctx, model.ContainerName, model.Port)
		if err == nil && healthy {
			model.MarkActive()
			log.Printf("Model %s is now active and healthy", modelID)
			return nil
		}

		if i < maxRetries-1 {
			time.Sleep(interval)
		}
	}

	model.MarkError()
	return fmt.Errorf("model failed to become healthy after %d retries", maxRetries)
}

// determineSleepLevel decides whether to use level 1 or level 2 sleep based on available system RAM
func (s *Switcher) determineSleepLevel(model *models.Model) int {
	availableRAMGB := s.ramFetcher.GetAvailableRAMGB()

	// If available RAM is greater than model's GPU memory requirement, use level 1
	// Otherwise, use level 2 to save RAM
	if availableRAMGB >= model.GPUMemoryGB {
		return 1 // Level 1: offload to CPU RAM
	}
	return 2 // Level 2: discard weights
}
