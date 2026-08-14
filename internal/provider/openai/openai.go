// Package openai adapts OpenAI-compatible /v1 endpoints to the Provider
// contracts. It is deliberately a transport adapter: it does not invoke tools,
// persist prompts or responses, or provide a fallback model.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

const defaultTimeout = 180 * time.Second

// Config identifies one configured deployment. APIKey is only used to form an
// Authorization header and is never included in returned errors.
type Config struct {
	BaseURL               string
	APIKey                string
	Model                 string
	DeploymentID          string
	ReliableContextTokens int
	HTTPClient            *http.Client
	Headers               http.Header
}

type Provider struct {
	baseURL               *url.URL
	apiKey                string
	model                 string
	deploymentID          string
	reliableContextTokens int
	client                *http.Client
	headers               http.Header
}

func New(config Config) (*Provider, error) {
	baseURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "OpenAI-compatible base URL must be an absolute http(s) URL")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "model is required")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &Provider{
		baseURL: baseURL, apiKey: config.APIKey, model: config.Model, deploymentID: config.DeploymentID,
		reliableContextTokens: config.ReliableContextTokens, client: client, headers: config.Headers.Clone(),
	}, nil
}

func (p *Provider) Chat(ctx context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	started := time.Now()
	if err := p.validateRequest(request); err != nil {
		return contracts.ChatResponse{}, err
	}
	body, err := p.requestBody(request, false)
	if err != nil {
		return contracts.ChatResponse{}, err
	}
	response, err := p.do(ctx, http.MethodPost, "chat/completions", body)
	if err != nil {
		return contracts.ChatResponse{}, err
	}
	defer response.Body.Close()
	if err := p.requireSuccess(response); err != nil {
		return contracts.ChatResponse{}, err
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024))
	if err != nil {
		if classified := classifyReadError(ctx, err); classified != nil {
			return contracts.ChatResponse{}, classified
		}
		return contracts.ChatResponse{}, contracts.NewError(contracts.ErrInvalidResponse, "provider response could not be read: "+err.Error())
	}
	var payload chatCompletion
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return contracts.ChatResponse{}, contracts.NewError(contracts.ErrInvalidResponse, malformedCompletionMessage(encoded))
	}
	result, err := normalizeCompletion(payload)
	if err != nil {
		return contracts.ChatResponse{}, err
	}
	result.Latency = time.Since(started)
	return result, nil
}

func (p *Provider) ChatStream(ctx context.Context, request contracts.ChatRequest, sink contracts.StreamSink) error {
	if sink == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "stream sink is required")
	}
	if err := p.validateRequest(request); err != nil {
		return err
	}
	body, err := p.requestBody(request, true)
	if err != nil {
		return err
	}
	started := time.Now()
	response, err := p.do(ctx, http.MethodPost, "chat/completions", body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err = p.requireSuccess(response); err != nil {
		return err
	}

	scanner := bufio.NewScanner(io.LimitReader(response.Body, 8*1024*1024))
	scanner.Buffer(make([]byte, 4*1024), 1024*1024)
	var text strings.Builder
	var messageID, finishReason string
	var usage contracts.TokenUsage
	calls := map[int]*contracts.ToolCall{}
	sequence := 0
	completed := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			completed = true
			break
		}
		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return contracts.NewError(contracts.ErrInvalidResponse, "provider returned malformed stream event")
		}
		if chunk.ID != "" {
			messageID = chunk.ID
		}
		usage = normalizeUsage(chunk.Usage)
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			if choice.Delta.Content != "" {
				text.WriteString(choice.Delta.Content)
				sequence++
				if err := sink(contracts.StreamEvent{Type: contracts.StreamEventTextDelta, TextDelta: choice.Delta.Content, Sequence: sequence}); err != nil {
					return err
				}
			}
			for _, delta := range choice.Delta.ToolCalls {
				call := calls[delta.Index]
				if call == nil {
					call = &contracts.ToolCall{Sequence: delta.Index, ProviderRawType: delta.Type}
					calls[delta.Index] = call
				}
				if delta.ID != "" {
					call.ID = delta.ID
				}
				if delta.Type != "" {
					call.ProviderRawType = delta.Type
				}
				if delta.Function.Name != "" {
					call.Name += delta.Function.Name
				}
				if delta.Function.Arguments != "" {
					call.ArgumentsJSON += delta.Function.Arguments
				}
				copy := *call
				sequence++
				if err := sink(contracts.StreamEvent{Type: contracts.StreamEventToolCallDelta, ToolCall: &copy, Sequence: sequence}); err != nil {
					return err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return transportError(ctx, err)
	}
	if !completed {
		return contracts.NewError(contracts.ErrInvalidResponse, "provider stream ended without a completion marker")
	}
	toolCalls, err := orderedToolCalls(calls)
	if err != nil {
		return err
	}
	result := contracts.ChatResponse{MessageID: messageID, Text: text.String(), ToolCalls: toolCalls, FinishReason: finishReason, Usage: usage, Latency: time.Since(started)}
	sequence++
	return sink(contracts.StreamEvent{Type: contracts.StreamEventCompleted, Response: &result, Sequence: sequence})
}

func (p *Provider) HealthCheck(ctx context.Context) contracts.HealthStatus {
	response, err := p.do(ctx, http.MethodGet, "models", nil)
	if err != nil {
		return contracts.HealthStatus{Healthy: false, Message: safeErrorMessage(err)}
	}
	defer response.Body.Close()
	if err = p.requireSuccess(response); err != nil {
		return contracts.HealthStatus{Healthy: false, Message: safeErrorMessage(err)}
	}
	return contracts.HealthStatus{Healthy: true, Message: "OpenAI-compatible endpoint is reachable"}
}

// ProbeCapabilities actively checks the text, streaming and tool request
// shapes. Each request is read-only to the provider, but callers should use a
// bounded context because a probe consumes model capacity.
func (p *Provider) ProbeCapabilities(ctx context.Context) contracts.CapabilitySnapshot {
	snapshot := contracts.CapabilitySnapshot{Version: contracts.SchemaVersion, ReliableContextTokens: p.reliableContextTokens, ProbedAt: time.Now().UTC()}
	if !p.HealthCheck(ctx).Healthy {
		return snapshot
	}
	probe := contracts.ChatRequest{DeploymentID: p.deploymentID, Messages: []contracts.Message{{Role: "user", Content: "Reply with OK."}}}
	if _, err := p.Chat(ctx, probe); err != nil {
		return snapshot
	}
	var receivedCompletion bool
	if err := p.ChatStream(ctx, probe, func(event contracts.StreamEvent) error {
		receivedCompletion = receivedCompletion || event.Type == contracts.StreamEventCompleted
		return nil
	}); err == nil && receivedCompletion {
		snapshot.SupportsStreaming = true
	}
	toolProbe := probe
	toolProbe.Tools = []contracts.ToolDefinition{{Version: contracts.SchemaVersion, Name: "probe_echo", Description: "Return the provided value without side effects.", ParametersSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"value": map[string]interface{}{"type": "string"}}}}}
	_, err := p.Chat(ctx, toolProbe)
	snapshot.SupportsTools = err == nil
	return snapshot
}

func (p *Provider) validateRequest(request contracts.ChatRequest) error {
	if p.deploymentID != "" && request.DeploymentID != "" && request.DeploymentID != p.deploymentID {
		return contracts.NewError(contracts.ErrInvalidInput, "request deployment does not match this provider")
	}
	if len(request.Messages) == 0 {
		return contracts.NewError(contracts.ErrInvalidInput, "at least one chat message is required")
	}
	for _, message := range request.Messages {
		switch message.Role {
		case "system", "user", "assistant", "tool":
		default:
			return contracts.NewError(contracts.ErrInvalidInput, "unsupported chat message role")
		}
	}
	return nil
}

func (p *Provider) requestBody(request contracts.ChatRequest, stream bool) ([]byte, error) {
	messages := make([]wireMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		calls := make([]wireToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.Name) == "" || !json.Valid([]byte(call.ArgumentsJSON)) {
				return nil, contracts.NewError(contracts.ErrInvalidToolCall, "tool call name and JSON arguments are required")
			}
			calls = append(calls, wireToolCall{ID: call.ID, Type: "function", Function: wireFunctionCall{Name: call.Name, Arguments: call.ArgumentsJSON}})
		}
		messages = append(messages, wireMessage{Role: message.Role, Content: message.Content, ToolCallID: message.ToolCallID, ToolCalls: calls})
	}
	tools := make([]wireTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, contracts.NewError(contracts.ErrInvalidInput, "tool name is required")
		}
		schema := tool.ParametersSchema
		if schema == nil {
			schema = map[string]interface{}{"type": "object", "additionalProperties": true}
		}
		tools = append(tools, wireTool{Type: "function", Function: wireFunction{Name: tool.Name, Description: tool.Description, Parameters: schema}})
	}
	requestBody := wireRequest{Model: p.model, Messages: messages, Tools: tools, Stream: stream}
	if request.JSONMode {
		requestBody.ResponseFormat = &wireResponseFormat{Type: "json_object"}
	}
	return json.Marshal(requestBody)
}

func (p *Provider) do(ctx context.Context, method, endpoint string, body []byte) (*http.Response, error) {
	target := *p.baseURL
	target.Path = strings.TrimRight(target.Path, "/") + "/" + endpoint
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "could not construct provider request")
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, values := range p.headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	if p.apiKey != "" && request.Header.Get("Authorization") == "" {
		request.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, transportError(ctx, err)
	}
	return response, nil
}

func (p *Provider) requireSuccess(response *http.Response) error {
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	message := redact(providerMessage(body), p.apiKey)
	switch response.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return contracts.NewError(contracts.ErrAuthenticationFailed, message)
	case http.StatusTooManyRequests:
		return contracts.NewError(contracts.ErrRateLimited, message)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return contracts.NewError(contracts.ErrRequestTimeout, message)
	case http.StatusBadRequest:
		if strings.Contains(strings.ToLower(message), "context") {
			return contracts.NewError(contracts.ErrContextOverflow, message)
		}
		return contracts.NewError(contracts.ErrInvalidInput, message)
	case http.StatusNotFound, http.StatusServiceUnavailable:
		return contracts.NewError(contracts.ErrModelUnavailable, message)
	default:
		return contracts.NewError(contracts.ErrProviderInternal, message)
	}
}

func transportError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return contracts.NewError(contracts.ErrRequestCancelled, "provider request was cancelled")
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return contracts.NewError(contracts.ErrRequestTimeout, "provider request timed out")
	}
	return &contracts.Error{Code: contracts.ErrEndpointUnreachable, Message: "provider endpoint is unreachable", Cause: err}
}

func safeErrorMessage(err error) string {
	if domain, ok := err.(*contracts.Error); ok {
		return domain.Message
	}
	return "provider endpoint is unreachable"
}

func providerMessage(body []byte) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && strings.TrimSpace(payload.Error.Message) != "" {
		return payload.Error.Message
	}
	return "provider returned an unsuccessful HTTP status"
}

func redact(value, secret string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[REDACTED]")
}

type wireRequest struct {
	Model          string              `json:"model"`
	Messages       []wireMessage       `json:"messages"`
	Tools          []wireTool          `json:"tools,omitempty"`
	Stream         bool                `json:"stream"`
	ResponseFormat *wireResponseFormat `json:"response_format,omitempty"`
}
type wireResponseFormat struct {
	Type string `json:"type"`
}
type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
}
type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}
type wireFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}
type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}
type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
type chatCompletion struct {
	ID      string             `json:"id"`
	Choices []completionChoice `json:"choices"`
	Usage   usage              `json:"usage"`
}
type completionChoice struct {
	Message      completionMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}
type completionMessage struct {
	Content   json.RawMessage `json:"content"`
	ToolCalls []wireToolCall  `json:"tool_calls"`
}
type chatCompletionChunk struct {
	ID      string        `json:"id"`
	Choices []chunkChoice `json:"choices"`
	Usage   usage         `json:"usage"`
}
type chunkChoice struct {
	Delta        chunkDelta `json:"delta"`
	FinishReason string     `json:"finish_reason"`
}
type chunkDelta struct {
	Content   string          `json:"content"`
	ToolCalls []chunkToolCall `json:"tool_calls"`
}
type chunkToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

func normalizeCompletion(payload chatCompletion) (contracts.ChatResponse, error) {
	if len(payload.Choices) == 0 {
		return contracts.ChatResponse{}, contracts.NewError(contracts.ErrInvalidResponse, "provider response has no choices")
	}
	choice := payload.Choices[0]
	text, err := contentString(choice.Message.Content)
	if err != nil {
		return contracts.ChatResponse{}, err
	}
	calls := make(map[int]*contracts.ToolCall, len(choice.Message.ToolCalls))
	for index, call := range choice.Message.ToolCalls {
		calls[index] = &contracts.ToolCall{ID: call.ID, Name: call.Function.Name, ArgumentsJSON: call.Function.Arguments, Sequence: index, ProviderRawType: call.Type}
	}
	toolCalls, err := orderedToolCalls(calls)
	if err != nil {
		return contracts.ChatResponse{}, err
	}
	return contracts.ChatResponse{MessageID: payload.ID, Text: text, ToolCalls: toolCalls, FinishReason: choice.FinishReason, Usage: normalizeUsage(payload.Usage)}, nil
}

func contentString(content json.RawMessage) (string, error) {
	if len(content) == 0 || string(content) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return text, nil
	}
	return "", contracts.NewError(contracts.ErrInvalidResponse, "provider response content must be text")
}

func malformedCompletionMessage(body []byte) string {
	preview := strings.TrimSpace(string(body))
	preview = strings.Join(strings.Fields(preview), " ")
	if len([]rune(preview)) > 180 {
		preview = string([]rune(preview)[:180]) + "…"
	}
	if preview == "" {
		return "provider returned an empty response body"
	}
	return "provider returned malformed chat completion JSON: " + preview
}
func normalizeUsage(value usage) contracts.TokenUsage {
	return contracts.TokenUsage{InputTokens: value.PromptTokens, OutputTokens: value.CompletionTokens}
}
func orderedToolCalls(calls map[int]*contracts.ToolCall) ([]contracts.ToolCall, error) {
	result := make([]contracts.ToolCall, 0, len(calls))
	for index := 0; index < len(calls); index++ {
		call := calls[index]
		if call == nil || call.Name == "" || !json.Valid([]byte(call.ArgumentsJSON)) {
			return nil, contracts.NewError(contracts.ErrInvalidToolCall, "provider returned an invalid tool call")
		}
		result = append(result, *call)
	}
	return result, nil
}

var _ contracts.ChatProvider = (*Provider)(nil)
