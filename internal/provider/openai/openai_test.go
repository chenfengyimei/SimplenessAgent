package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestChatNormalizesTextToolsUsageAndRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Fatalf("unexpected authorization %q", got)
		}
		var body wireRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "local-model" || body.Stream || len(body.Tools) != 1 || body.Tools[0].Function.Parameters["type"] != "object" {
			t.Fatalf("unexpected request %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chat_1","choices":[{"message":{"content":"Done","tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_files","arguments":"{\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":5}}`))
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL+"/v1")
	response, err := provider.Chat(context.Background(), contracts.ChatRequest{
		DeploymentID: "dep-local",
		Messages:     []contracts.Message{{Role: "user", Content: "Inspect files"}},
		Tools:        []contracts.ToolDefinition{{Name: "list_files", ParametersSchema: map[string]interface{}{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.MessageID != "chat_1" || response.Text != "Done" || response.FinishReason != "tool_calls" || response.Usage.InputTokens != 12 || response.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected response %#v", response)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].ArgumentsJSON != `{"path":"."}` {
		t.Fatalf("tool call was not normalized: %#v", response.ToolCalls)
	}
}

func TestChatRequestsJSONModeAndIncludesSafeMalformedPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body wireRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.ResponseFormat == nil || body.ResponseFormat.Type != "json_object" {
			t.Fatalf("JSON mode was not forwarded: %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte("<html>gateway page</html>"))
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL)
	_, err := provider.Chat(context.Background(), contracts.ChatRequest{Messages: []contracts.Message{{Role: "user", Content: "json"}}, JSONMode: true})
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrInvalidResponse || !strings.Contains(domain.Message, "gateway page") {
		t.Fatalf("expected actionable malformed response error, got %#v", err)
	}
}

func TestChatStreamEmitsOrderedDeltasAndCompletedAggregate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		events := []string{
			`data: {"id":"chat_stream","choices":[{"delta":{"content":"Hel"}}]}`,
			`data: {"id":"chat_stream","choices":[{"delta":{"content":"lo","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_","arguments":"{\"path\":\""}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"file","arguments":"README.md\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`,
			"data: [DONE]",
		}
		for _, event := range events {
			_, _ = fmt.Fprint(writer, event+"\n\n")
			flusher.Flush()
		}
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL+"/v1")
	var events []contracts.StreamEvent
	err := provider.ChatStream(context.Background(), contracts.ChatRequest{Messages: []contracts.Message{{Role: "user", Content: "Read README"}}}, func(event contracts.StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("unexpected stream events %#v", events)
	}
	for index, event := range events {
		if event.Sequence != index+1 {
			t.Fatalf("events are not ordered: %#v", events)
		}
	}
	completed := events[len(events)-1]
	if completed.Type != contracts.StreamEventCompleted || completed.Response.Text != "Hello" || completed.Response.Usage.InputTokens != 7 {
		t.Fatalf("completion aggregate missing: %#v", completed)
	}
	if len(completed.Response.ToolCalls) != 1 || completed.Response.ToolCalls[0].Name != "read_file" || completed.Response.ToolCalls[0].ArgumentsJSON != `{"path":"README.md"}` {
		t.Fatalf("stream tool call not aggregated: %#v", completed.Response.ToolCalls)
	}
}

func TestProviderClassifiesCancellationWithoutLeakingCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := provider.Chat(ctx, contracts.ChatRequest{Messages: []contracts.Message{{Role: "user", Content: "hello"}}})
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrRequestCancelled {
		t.Fatalf("unexpected cancellation error: %#v", err)
	}
	if strings.Contains(err.Error(), "test-secret") {
		t.Fatalf("credential leaked into error: %v", err)
	}
}

func TestProviderRedactsConfiguredCredentialFromProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"error":{"message":"bad credential test-secret"}}`))
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL)
	_, err := provider.Chat(context.Background(), contracts.ChatRequest{Messages: []contracts.Message{{Role: "user", Content: "hello"}}})
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrAuthenticationFailed {
		t.Fatalf("unexpected provider error: %#v", err)
	}
	if strings.Contains(err.Error(), "test-secret") || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("credential was not redacted: %v", err)
	}
}

func TestProbeCapabilitiesActivelyTestsSupportedShapes(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/models" {
			_, _ = writer.Write([]byte(`{"data":[]}`))
			return
		}
		if request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		mu.Lock()
		requests++
		mu.Unlock()
		var body wireRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Stream {
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(writer, "data: {\"id\":\"stream\",\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chat","choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL+"/v1")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot := provider.ProbeCapabilities(ctx)
	if !snapshot.SupportsStreaming || !snapshot.SupportsTools || snapshot.ProbedAt.IsZero() {
		t.Fatalf("unexpected capability snapshot %#v", snapshot)
	}
	mu.Lock()
	count := requests
	mu.Unlock()
	if count != 3 {
		t.Fatalf("expected text, stream and tool probes, got %d", count)
	}
}

func newTestProvider(t *testing.T, baseURL string) *Provider {
	t.Helper()
	provider, err := New(Config{BaseURL: baseURL, APIKey: "test-secret", Model: "local-model", DeploymentID: "dep-local", ReliableContextTokens: 8192})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestChatClassifiesReadTimeoutAsRequestTimeout(t *testing.T) {
	handlerDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
		select {
		case <-request.Context().Done():
		case <-handlerDone:
		}
	}))
	defer func() {
		close(handlerDone)
		server.Close()
	}()
	provider, err := New(Config{BaseURL: server.URL, APIKey: "test-secret", Model: "local-model", DeploymentID: "dep-local", ReliableContextTokens: 8192, HTTPClient: &http.Client{Timeout: 50 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Chat(context.Background(), contracts.ChatRequest{Messages: []contracts.Message{{Role: "user", Content: "hello"}}})
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrRequestTimeout {
		t.Fatalf("expected ErrRequestTimeout for read timeout, got: %v", err)
	}
}

func TestChatPreservesUnderlyingReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		if hijacker, ok := writer.(http.Hijacker); ok {
			conn, _, _ := hijacker.Hijack()
			conn.Close()
		}
	}))
	defer server.Close()
	provider := newTestProvider(t, server.URL)
	_, err := provider.Chat(context.Background(), contracts.ChatRequest{Messages: []contracts.Message{{Role: "user", Content: "hello"}}})
	var domain *contracts.Error
	if !errors.As(err, &domain) {
		t.Fatalf("expected contracts.Error, got: %v", err)
	}
	if domain.Code != contracts.ErrEndpointUnreachable && domain.Code != contracts.ErrInvalidResponse {
		t.Fatalf("expected endpoint unreachable or invalid response for connection drop, got: %s", domain.Code)
	}
	if domain.Message == "provider response could not be read" {
		t.Fatalf("generic message was not replaced with diagnostic detail: %s", domain.Message)
	}
}
