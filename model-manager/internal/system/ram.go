package system

import (
	"log"
	"os"
	"strconv"
	"strings"
)

// RAMFetcher provides system RAM information
type RAMFetcher interface {
	GetAvailableRAMGB() float64
}

// ProcMemInfoFetcher reads RAM from /proc/meminfo
type ProcMemInfoFetcher struct{}

// NewRAMFetcher creates a new RAM fetcher for the current OS
func NewRAMFetcher() RAMFetcher {
	return &ProcMemInfoFetcher{}
}

// GetAvailableRAMGB returns available system RAM in GB
func (f *ProcMemInfoFetcher) GetAvailableRAMGB() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("Warning: could not read /proc/meminfo: %v, defaulting to 0 GB", err)
		return 0
	}

	// Parse MemAvailable from /proc/meminfo
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				if kb, err := strconv.ParseFloat(fields[1], 64); err == nil {
					return kb / 1024 / 1024 // Convert KB to GB
				}
			}
		}
	}

	log.Printf("Warning: could not parse MemAvailable from /proc/meminfo, defaulting to 0 GB")
	return 0
}

// MockRAMFetcher is a test implementation that returns a fixed value
type MockRAMFetcher struct {
	AvailableRAMGB float64
}

// GetAvailableRAMGB returns the mocked RAM value
func (f *MockRAMFetcher) GetAvailableRAMGB() float64 {
	return f.AvailableRAMGB
}
