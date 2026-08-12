package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

type proposalProvider struct {
	responses []contracts.ChatResponse
}

func (p *proposalProvider) Chat(_ context.Context, _ contracts.ChatRequest) (contracts.ChatResponse, error) {
	if len(p.responses) == 0 {
		return contracts.ChatResponse{}, errors.New("unexpected model call")
	}
	result := p.responses[0]
	p.responses = p.responses[1:]
	return result, nil
}
func (*proposalProvider) ChatStream(context.Context, contracts.ChatRequest, contracts.StreamSink) error {
	return errors.New("not used")
}
func (*proposalProvider) HealthCheck(context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true}
}
func (*proposalProvider) ProbeCapabilities(context.Context) contracts.CapabilitySnapshot {
	return contracts.CapabilitySnapshot{ProbedAt: time.Now().UTC()}
}

func TestModelWriteProposalWaitsForApprovalThenWritesAtomically(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &proposalProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "proposal", Name: "propose_write_file", ArgumentsJSON: `{"path":"note.txt","content":"after","expected_content_hash":"` + hashText("before") + `"}`}}}}}
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "DESKTOP", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "write", Goal: "replace note", AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "exists", Type: contracts.AcceptanceFileExists, Description: "note exists", Spec: map[string]interface{}{"path": "note.txt"}}}})
	if err != nil {
		t.Fatal(err)
	}
	planVersion := writeProposalPlan(created, "write-step")
	event, err := service.newEvent(ctx, created.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": planVersion.PlanID, "revision": planVersion.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.store.CreatePlan(ctx, planVersion, event); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunModelStep(ctx, RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || snapshot.Task.Status != contracts.TaskWaitingApproval || snapshot.Steps[0].Status != contracts.StepWaitingApproval {
		t.Fatal(snapshot, err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "before" {
		t.Fatal(string(content), err)
	}
	artifacts, err := service.ListTaskArtifacts(ctx, created.ID)
	foundProposal := false
	for _, artifact := range artifacts {
		foundProposal = foundProposal || artifact.Kind == "PENDING_WRITE"
	}
	if err != nil || !foundProposal {
		t.Fatal(artifacts, err)
	}
	completed, err := service.ApprovePendingWrite(ctx, created.ID, "write-step", time.Now().Add(time.Minute))
	if err != nil || completed.Task.Status != contracts.TaskCompleted {
		t.Fatal(completed, err)
	}
	content, err = os.ReadFile(path)
	if err != nil || string(content) != "after" {
		t.Fatal(string(content), err)
	}
}

func TestModelWriteProposalRejectsChangedFileWithoutOverwriting(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &proposalProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "proposal", Name: "propose_write_file", ArgumentsJSON: `{"path":"note.txt","content":"after","expected_content_hash":"` + hashText("before") + `"}`}}}}}
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "DESKTOP", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "write", Goal: "replace note", AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "exists", Type: contracts.AcceptanceFileExists, Description: "note exists", Spec: map[string]interface{}{"path": "note.txt"}}}})
	if err != nil {
		t.Fatal(err)
	}
	planVersion := writeProposalPlan(created, "write-step")
	event, err := service.newEvent(ctx, created.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": planVersion.PlanID, "revision": planVersion.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.store.CreatePlan(ctx, planVersion, event); err != nil {
		t.Fatal(err)
	}
	if _, err = service.RunModelStep(ctx, RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte("changed elsewhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = service.ApprovePendingWrite(ctx, created.ID, "write-step", time.Now().Add(time.Minute)); err == nil {
		t.Fatal("approval must reject a changed target")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "changed elsewhere" {
		t.Fatal(string(content), err)
	}
}

func writeProposalPlan(item contracts.Task, stepID string) contracts.PlanVersion {
	step := contracts.StepSpec{Version: contracts.SchemaVersion, StepID: stepID, Title: "Propose note change", Goal: "change the note after approval", AllowedTools: []string{"propose_write_file"}, WorkspaceScopes: []string{"."}, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "exists", Type: contracts.AcceptanceFileExists, Description: "note exists", Spec: map[string]interface{}{"path": "note.txt"}}}, Risk: contracts.RiskWrite, Budget: contracts.StepBudget{MaxAttempts: 1, MaxIterations: 2, MaxDurationMS: 1000, MaxInputTokens: 1000, MaxOutputTokens: 1000}, ExecutionMode: "AGENT"}
	return contracts.PlanVersion{Version: contracts.SchemaVersion, PlanID: "proposal-plan-" + stepID, TaskID: item.ID, Revision: 2, ParentPlanID: "initial", Reason: "TEST", Summary: "write proposal", Steps: []contracts.StepSpec{step}, CreatedByAgent: "test", CreatedAt: time.Now().UTC()}
}

func hashText(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}
