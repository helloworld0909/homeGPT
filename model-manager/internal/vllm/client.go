package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps HTTP calls to vLLM servers
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new vLLM client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Health checks if the vLLM server is healthy
func (c *Client) Health(ctx context.Context, host string, port int) (bool, error) {
	url := fmt.Sprintf("http://%s:%d/health", host, port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// IsSleeping checks if the vLLM server is in sleep mode
func (c *Client) IsSleeping(ctx context.Context, host string, port int) (bool, error) {
	url := fmt.Sprintf("http://%s:%d/is_sleeping", host, port)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	var result map[string]bool
	if err := json.Unmarshal(body, &result); err != nil {
		return false, err
	}

	return result["is_sleeping"], nil
}

// Sleep puts the vLLM server to sleep
func (c *Client) Sleep(ctx context.Context, host string, port int, level int) error {
	url := fmt.Sprintf("http://%s:%d/sleep?level=%d", host, port, level)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sleep failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// WakeUp wakes up the vLLM server from sleep
func (c *Client) WakeUp(ctx context.Context, host string, port int) error {
	url := fmt.Sprintf("http://%s:%d/wake_up", host, port)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("wake_up failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
