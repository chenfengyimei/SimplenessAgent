package contracts

import (
	"context"
	"time"
)

// ChatProvider is the sole model boundary used by Agent Core. Provider-specific
// wire formats must be normalized before reaching tools or scheduling.
type ChatProvider interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	HealthCheck(ctx context.Context) HealthStatus
	ProbeCapabilities(ctx context.Context) CapabilitySnapshot
}

type ChatRequest struct {
	DeploymentID string           `json:"deployment_id"`
	Messages     []Message        `json:"messages"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	MessageID    string        `json:"message_id"`
	Text         string        `json:"text"`
	FinishReason string        `json:"finish_reason"`
	Usage        TokenUsage    `json:"usage"`
	Latency      time.Duration `json:"latency"`
}

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type HealthStatus struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
}

type CapabilitySnapshot struct {
	Version               int       `json:"version"`
	SupportsTools         bool      `json:"supports_tools"`
	SupportsStreaming     bool      `json:"supports_streaming"`
	SupportsTokenCount    bool      `json:"supports_token_count"`
	ReliableContextTokens int       `json:"reliable_context_tokens"`
	ProbedAt              time.Time `json:"probed_at"`
}
