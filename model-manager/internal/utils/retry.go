package utils

import (
	"context"
	"time"
)

// RetryConfig configures exponential backoff retry behavior
type RetryConfig struct {
	MaxAttempts  int           // Maximum number of retry attempts
	InitialDelay time.Duration // Initial delay before first retry
	MaxDelay     time.Duration // Maximum delay between retries (caps exponential growth)
	Multiplier   float64       // Multiplier for exponential backoff (typically 2.0)
}

// DefaultRetryConfig returns a sensible default configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1000 * time.Millisecond,
		Multiplier:   2.0,
	}
}

// RetryWithBackoff retries a function with exponential backoff until it succeeds or max attempts reached
func RetryWithBackoff(ctx context.Context, cfg RetryConfig, fn func() error) error {
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Check context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Try the operation
		if err := fn(); err == nil {
			return nil // Success
		} else {
			lastErr = err
		}

		// Don't sleep after last attempt
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		// Sleep with exponential backoff
		select {
		case <-time.After(delay):
			// Calculate next delay with exponential growth
			delay = time.Duration(float64(delay) * cfg.Multiplier)
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

// PollUntil polls a condition function with exponential backoff until it returns true
func PollUntil(ctx context.Context, cfg RetryConfig, condition func() (bool, error)) error {
	return RetryWithBackoff(ctx, cfg, func() error {
		ok, err := condition()
		if err != nil {
			return err
		}
		if !ok {
			return &retryableError{msg: "condition not met"}
		}
		return nil
	})
}

// retryableError is an internal error type for polling
type retryableError struct {
	msg string
}

func (e *retryableError) Error() string {
	return e.msg
}
