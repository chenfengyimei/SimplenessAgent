// Package mock supplies a deterministic Provider for contracts and integration
// tests. It deliberately has no hidden fallback path.
package mock

import (
	"context"
	"time"

	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/pkg/contracts"
)

type Provider struct{ Response string }

func (p Provider) Chat(_ context.Context, _ contracts.ChatRequest) (contracts.ChatResponse, error) {
	return contracts.ChatResponse{MessageID: task.NewID("msg"), Text: p.Response, FinishReason: "stop", Latency: 0}, nil
}
func (p Provider) ChatStream(ctx context.Context, req contracts.ChatRequest, sink contracts.StreamSink) error {
	response, err := p.Chat(ctx, req)
	if err != nil {
		return err
	}
	if response.Text != "" {
		if err = sink(contracts.StreamEvent{Type: contracts.StreamEventTextDelta, TextDelta: response.Text, Sequence: 1}); err != nil {
			return err
		}
	}
	return sink(contracts.StreamEvent{Type: contracts.StreamEventCompleted, Response: &response, Sequence: 2})
}
func (p Provider) HealthCheck(_ context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true, Message: "mock provider is ready"}
}
func (p Provider) ProbeCapabilities(_ context.Context) contracts.CapabilitySnapshot {
	return contracts.CapabilitySnapshot{Version: contracts.SchemaVersion, SupportsTools: true, SupportsStreaming: false, SupportsTokenCount: false, ReliableContextTokens: 4096, ProbedAt: time.Now().UTC()}
}
