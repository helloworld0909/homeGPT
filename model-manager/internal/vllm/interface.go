package vllm

import "context"

// VLLMClient is an interface for vLLM operations, allowing for mocking in tests
type VLLMClient interface {
	Health(ctx context.Context, host string, port int) (bool, error)
	IsSleeping(ctx context.Context, host string, port int) (bool, error)
	Sleep(ctx context.Context, host string, port int, level int) error
	WakeUp(ctx context.Context, host string, port int) error
}

// Ensure Client implements VLLMClient interface
var _ VLLMClient = (*Client)(nil)
