package contracts

import (
	"context"
	"time"
)

// ChatProvider is the sole model boundary used by Agent Core. Provider-specific
// wire formats must be normalized before reaching tools or scheduling.
type ChatProvider interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest, sink StreamSink) error
	HealthCheck(ctx context.Context) HealthStatus
	ProbeCapabilities(ctx context.Context) CapabilitySnapshot
}

type ChatRequest struct {
	DeploymentID string           `json:"deployment_id"`
	Messages     []Message        `json:"messages"`
	Tools        []ToolDefinition `json:"tools,omitempty"`
	JSONMode     bool             `json:"json_mode,omitempty"`
	// MaxOutputTokens is a provider-facing ceiling for one response. Keeping
	// this on the request (rather than merely checking usage afterwards) is
	// essential for local/small models: it leaves room for the next tool turn.
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ChatResponse struct {
	MessageID    string        `json:"message_id"`
	Text         string        `json:"text"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	FinishReason string        `json:"finish_reason"`
	Usage        TokenUsage    `json:"usage"`
	Latency      time.Duration `json:"latency"`
}

// ToolCall is the provider-neutral representation of a model-requested tool.
// ArgumentsJSON is preserved as JSON text so the Worker can validate it against
// the registered tool schema before decoding or invoking anything.
type ToolCall struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ArgumentsJSON   string `json:"arguments_json"`
	Sequence        int    `json:"sequence"`
	ProviderRawType string `json:"provider_raw_type,omitempty"`
}

type StreamEventType string

const (
	StreamEventTextDelta     StreamEventType = "TEXT_DELTA"
	StreamEventToolCallDelta StreamEventType = "TOOL_CALL_DELTA"
	StreamEventCompleted     StreamEventType = "COMPLETED"
)

// StreamEvent is emitted in response order. COMPLETED contains the normalized
// aggregate, while deltas let a caller render or persist progress incrementally.
type StreamEvent struct {
	Type      StreamEventType `json:"type"`
	TextDelta string          `json:"text_delta,omitempty"`
	ToolCall  *ToolCall       `json:"tool_call,omitempty"`
	Response  *ChatResponse   `json:"response,omitempty"`
	Sequence  int             `json:"sequence"`
}

type StreamSink func(StreamEvent) error

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type HealthStatus struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
}

type CapabilitySnapshot struct {
	ID                    string    `json:"capability_snapshot_id,omitempty"`
	DeploymentID          string    `json:"deployment_id,omitempty"`
	Version               int       `json:"version"`
	SupportsTools         bool      `json:"supports_tools"`
	SupportsStreaming     bool      `json:"supports_streaming"`
	SupportsTokenCount    bool      `json:"supports_token_count"`
	ReliableContextTokens int       `json:"reliable_context_tokens"`
	ProbedAt              time.Time `json:"probed_at"`
}
