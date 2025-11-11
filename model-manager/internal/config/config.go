package config

import (
	"fmt"
	"os"

	"github.com/zheng/homeGPT/pkg/models"
	"gopkg.in/yaml.v3"
)

// Load reads and parses the configuration file
func Load(path string) (*models.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg models.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if len(cfg.Models) == 0 {
		return nil, fmt.Errorf("no models defined in config")
	}

	defaultCount := 0
	for i := range cfg.Models {
		if cfg.Models[i].Default {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		return nil, fmt.Errorf("exactly one model must be set as default, found %d", defaultCount)
	}

	return &cfg, nil
}
