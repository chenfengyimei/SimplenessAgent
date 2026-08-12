package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/internal/provider/mock"
	"github.com/xm/simplenessagent/pkg/contracts"
)

type memoryCredentialStore struct{ values map[string]string }

type scriptedChatProvider struct {
	mu        sync.Mutex
	responses []contracts.ChatResponse
	requests  []contracts.ChatRequest
}

func (p *scriptedChatProvider) Chat(_ context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests = append(p.requests, request)
	index := len(p.requests) - 1
	if index >= len(p.responses) {
		index = len(p.responses) - 1
	}
	return p.responses[index], nil
}
func (p *scriptedChatProvider) ChatStream(ctx context.Context, request contracts.ChatRequest, sink contracts.StreamSink) error {
	response, err := p.Chat(ctx, request)
	if err != nil {
		return err
	}
	return sink(contracts.StreamEvent{Type: contracts.StreamEventCompleted, Response: &response})
}
func (*scriptedChatProvider) HealthCheck(context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true}
}
func (*scriptedChatProvider) ProbeCapabilities(context.Context) contracts.CapabilitySnapshot {
	return contracts.CapabilitySnapshot{Version: contracts.SchemaVersion, SupportsTools: true, ProbedAt: time.Now().UTC()}
}

func (s *memoryCredentialStore) Save(id, key string) error      { s.values[id] = key; return nil }
func (s *memoryCredentialStore) Load(id string) (string, error) { return s.values[id], nil }
func (s *memoryCredentialStore) Delete(id string) error         { delete(s.values, id); return nil }

func TestDesktopTaskUsesAgentReportAcceptance(t *testing.T) {
	service, err := app.Open(context.Background(), app.Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	desktop := &App{ctx: context.Background(), service: service}
	workspace, err := desktop.CreateWorkspace("demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := desktop.CreateTask(workspace.ID, "inspect", "inspect workspace")
	if err != nil {
		t.Fatal(err)
	}
	criteria := snapshot.Task.Spec.AcceptanceCriteria
	if len(criteria) != 1 || criteria[0].Type != contracts.AcceptanceEvidenceExists || criteria[0].Spec["kind"] != "AGENT_REPORT" {
		t.Fatalf("unexpected desktop task acceptance: %#v", criteria)
	}
}

func TestConversationContinuesWithoutCreatingNewRoot(t *testing.T) {
	service, err := app.Open(context.Background(), app.Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: "not a plan"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	desktop := &App{ctx: context.Background(), service: service, apiKeys: map[string]string{}, credentials: &memoryCredentialStore{values: map[string]string{}}}
	workspace, err := desktop.CreateWorkspace("demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "DESKTOP", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := desktop.StartConversation(workspace.ID, "inspect the workspace", deployment.ID)
	if err != nil || conversation.Conversation.ID == "" || len(conversation.Messages) != 2 || len(conversation.Turns) != 1 || conversation.Turns[0].Snapshot.Task.Status != contracts.TaskCompleted {
		t.Fatal(conversation, err)
	}
	conversation, err = desktop.SendConversationMessage(conversation.Conversation.ID, "inspect again", deployment.ID)
	if err != nil || len(conversation.Messages) != 4 || len(conversation.Turns) != 2 {
		t.Fatal(conversation, err)
	}
	roots, err := desktop.ListConversations()
	if err != nil || len(roots) != 1 {
		t.Fatal(roots, err)
	}
}

func TestConversationRunsModelToolLoopAndReturnsModelReply(t *testing.T) {
	provider := &scriptedChatProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{{ID: "call-list", Name: "list_files", ArgumentsJSON: `{"path":".","limit":20}`}}},
		{Text: "我已查看工作目录，当前没有需要进一步读取的文件。"},
	}}
	service, err := app.Open(context.Background(), app.Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	desktop := &App{ctx: context.Background(), service: service, apiKeys: map[string]string{}, credentials: &memoryCredentialStore{values: map[string]string{}}}
	workspace, err := desktop.CreateWorkspace("demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "DESKTOP", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := desktop.StartConversation(workspace.ID, "请查看工作目录并告诉我有什么", deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || len(provider.requests[0].Tools) != 3 {
		t.Fatalf("expected model tool loop, got %#v", provider.requests)
	}
	if len(conversation.Messages) != 2 || conversation.Messages[1].Content != "我已查看工作目录，当前没有需要进一步读取的文件。" {
		t.Fatalf("expected model response in conversation: %#v", conversation.Messages)
	}
	if len(conversation.Turns) != 1 || conversation.Turns[0].Snapshot.Task.Status != contracts.TaskCompleted || conversation.Turns[0].Report.ToolName != "列出文件" {
		t.Fatalf("expected completed model turn with tool report: %#v", conversation.Turns)
	}
}

func TestConversationExecutionReceivesRecentTurnsAndRelevantMemory(t *testing.T) {
	provider := &scriptedChatProvider{responses: []contracts.ChatResponse{
		{Text: "我已收到这条初始消息。"},
		{Text: "你之前说的是初始需求。"},
	}}
	service, err := app.Open(context.Background(), app.Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	desktop := &App{ctx: context.Background(), service: service, apiKeys: map[string]string{}, credentials: &memoryCredentialStore{values: map[string]string{}}}
	workspace, err := desktop.CreateWorkspace("demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateMemory(context.Background(), contracts.MemoryRecord{Type: "PREFERENCE", WorkspaceID: workspace.ID, Title: "答复语言", Content: "始终使用中文回答。", SourceEventIDs: []string{"evt_user_preference"}, Confidence: 1, Importance: 1, Status: "PINNED", CreatedBy: "USER_CONFIRMED"})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "DESKTOP", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := desktop.StartConversation(workspace.ID, "初始需求：请记住这一点", deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = desktop.SendConversationMessage(conversation.Conversation.ID, "我之前说了什么？", deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected two model requests, got %d", len(provider.requests))
	}
	context := provider.requests[1].Messages[1].Content
	for _, expected := range []string{"RECENT_CONVERSATION", "初始需求：请记住这一点", "我已收到这条初始消息。", "MEMORY_PREFERENCE", "始终使用中文回答。"} {
		if !strings.Contains(context, expected) {
			t.Fatalf("second-turn context does not include %q:\n%s", expected, context)
		}
	}
}

func TestDesktopDeploymentNeverPersistsAPIKey(t *testing.T) {
	service, err := app.Open(context.Background(), app.Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	credentials := &memoryCredentialStore{values: map[string]string{}}
	desktop := &App{ctx: context.Background(), service: service, apiKeys: map[string]string{}, credentials: credentials}
	deployment, err := desktop.ConfigureOpenAICompatibleDeployment("local", "http://127.0.0.1:8080/v1", "model", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if deployment.CredentialRef != "windows-credential-manager" || desktop.apiKeys[deployment.ID] != "secret-value" {
		t.Fatal("desktop should retain the API key in the Windows credential manager")
	}
	items, err := desktop.ListDeployments()
	if err != nil || len(items) != 1 || items[0].CredentialRef != "windows-credential-manager" {
		t.Fatal(items, err)
	}
	if key, err := credentials.Load(deployment.ID); err != nil || key != "secret-value" {
		t.Fatalf("credential manager did not retain the API key: %q, %v", key, err)
	}
}
