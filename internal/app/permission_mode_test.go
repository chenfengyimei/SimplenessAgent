package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestPermissionModePlanOnlyRegistersWorkspaceReadTools(t *testing.T) {
	provider := &recordingPermissionProvider{responses: []contracts.ChatResponse{{Text: "已完成只读规划。"}}}
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(context.Background(), "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "TEST", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, planVersion, err := service.CreateTask(context.Background(), CreateTaskInput{WorkspaceID: workspace.ID, Title: "plan", Goal: "inspect", PermissionMode: contracts.PermissionModePlan, AcceptanceCriteria: agentAcceptance()})
	if err != nil {
		t.Fatal(err)
	}
	if created.Spec.PermissionProfileID != string(contracts.PermissionModePlan) || planVersion.Steps[0].Risk != contracts.RiskRead || contains(planVersion.Steps[0].AllowedTools, "run_go_test") || contains(planVersion.Steps[0].AllowedTools, "propose_write_file") {
		t.Fatalf("plan mode has unexpected plan: %#v", planVersion)
	}
	if _, err = service.RunModelStep(context.Background(), RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID}); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 {
		t.Fatal(provider.requests)
	}
	for _, definition := range provider.requests[0].Tools {
		if definition.Name == "run_go_test" || definition.Name == "write_file" || definition.Name == "propose_write_file" {
			t.Fatalf("plan mode exposed a forbidden tool: %#v", provider.requests[0].Tools)
		}
	}
}

func TestEditModeProjectCommandWaitsForApproval(t *testing.T) {
	provider := &recordingPermissionProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "command", Name: "propose_project_command", ArgumentsJSON: `{"command":"go_test","arguments":["."],"timeout_ms":5000}`}}}}}
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	root := t.TempDir()
	if err = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/edit-command\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "main.go"), []byte("package editcommand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := service.CreateWorkspace(context.Background(), "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "TEST", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, planVersion, err := service.CreateTask(context.Background(), CreateTaskInput{WorkspaceID: workspace.ID, Title: "edit", Goal: "test", PermissionMode: contracts.PermissionModeEdit, AllowWriteProposals: true, AcceptanceCriteria: agentAcceptance()})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunModelStep(context.Background(), RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || snapshot.Task.Status != contracts.TaskWaitingApproval {
		t.Fatal(snapshot, err)
	}
	pending, err := service.PendingCommand(context.Background(), created.ID)
	if err != nil || pending.Command != "go_test" || len(pending.Arguments) != 1 || pending.Arguments[0] != "." {
		t.Fatal(pending, err)
	}
	if len(provider.requests) != 1 {
		t.Fatal("command must pause before a second provider response", provider.requests)
	}
	completed, err := service.ApprovePendingCommand(context.Background(), created.ID, planVersion.Steps[0].StepID, time.Now().Add(time.Minute))
	if err != nil || completed.Task.Status != contracts.TaskCompleted {
		t.Fatal(completed, err)
	}
	if !hasEvent(completed.Events, "TOOL_APPROVED") || !hasEvent(completed.Events, "TOOL_INTENT_RECORDED") {
		t.Fatalf("command approval is not fully auditable: %#v", completed.Events)
	}
}

func TestDevelopmentModeRunsBoundedCommandWithWriteAheadIntent(t *testing.T) {
	provider := &recordingPermissionProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "test", Name: "run_project_command", ArgumentsJSON: `{"command":"go_test","arguments":["."],"timeout_ms":5000}`}}}, {Text: "测试已通过。"}}}
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	root := t.TempDir()
	if err = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/development-command\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "main.go"), []byte("package developmentcommand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := service.CreateWorkspace(context.Background(), "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "TEST", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(context.Background(), CreateTaskInput{WorkspaceID: workspace.ID, Title: "dev-command", Goal: "test", PermissionMode: contracts.PermissionModeDevelopment, AcceptanceCriteria: agentAcceptance()})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunModelStep(context.Background(), RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || snapshot.Task.Status != contracts.TaskCompleted {
		t.Fatal(snapshot, err)
	}
	if !hasEvent(snapshot.Events, "TOOL_INTENT_RECORDED") || !hasTool(provider.requests[0].Tools, "run_project_command") {
		t.Fatalf("development command was not bounded and audited: %#v", snapshot)
	}
}

func TestDevelopmentModeRegistersDirectToolsAndAuditsWriteIntent(t *testing.T) {
	provider := &recordingPermissionProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "write", Name: "write_file", ArgumentsJSON: `{"path":"note.txt","content":"after","expected_content_hash":"sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`}}}, {Text: "文件已创建。"}}}
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	root := t.TempDir()
	workspace, err := service.CreateWorkspace(context.Background(), "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "TEST", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(context.Background(), CreateTaskInput{WorkspaceID: workspace.ID, Title: "dev", Goal: "write", PermissionMode: contracts.PermissionModeDevelopment, AcceptanceCriteria: agentAcceptance()})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunModelStep(context.Background(), RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || snapshot.Task.Status != contracts.TaskCompleted {
		t.Fatal(snapshot, err)
	}
	if _, err = os.Stat(filepath.Join(root, "note.txt")); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || !hasTool(provider.requests[0].Tools, "write_file") || hasTool(provider.requests[0].Tools, "propose_write_file") {
		t.Fatalf("development mode tool surface is wrong: %#v", provider.requests)
	}
}

func agentAcceptance() []contracts.AcceptanceCriterion {
	return []contracts.AcceptanceCriterion{{ID: "agent", Type: contracts.AcceptanceEvidenceExists, Description: "agent", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}
}

func hasTool(definitions []contracts.ToolDefinition, name string) bool {
	for _, definition := range definitions {
		if definition.Name == name {
			return true
		}
	}
	return false
}

func hasEvent(events []contracts.EventEnvelope, wanted string) bool {
	for _, event := range events {
		if event.EventType == wanted {
			return true
		}
	}
	return false
}

type recordingPermissionProvider struct {
	responses []contracts.ChatResponse
	requests  []contracts.ChatRequest
}

func (p *recordingPermissionProvider) Chat(_ context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return contracts.ChatResponse{}, errors.New("unexpected model call")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}
func (*recordingPermissionProvider) ChatStream(context.Context, contracts.ChatRequest, contracts.StreamSink) error {
	return errors.New("not used")
}
func (*recordingPermissionProvider) HealthCheck(context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true}
}
func (*recordingPermissionProvider) ProbeCapabilities(context.Context) contracts.CapabilitySnapshot {
	return contracts.CapabilitySnapshot{ProbedAt: time.Now().UTC()}
}
