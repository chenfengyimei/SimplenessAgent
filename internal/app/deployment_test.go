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

	"github.com/xm/simplenessagent/internal/provider/mock"
	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestDeploymentProbePersistsCapabilitySnapshot(t *testing.T) {
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(deployment contracts.Deployment) (contracts.ChatProvider, error) { return mock.Provider{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "local", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1:8080/v1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	health, snapshot, err := service.ProbeDeployment(context.Background(), deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !health.Healthy || snapshot.ID == "" || snapshot.DeploymentID != deployment.ID {
		t.Fatalf("unexpected probe: %#v %#v", health, snapshot)
	}
	deployments, err := service.ListDeployments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 1 || deployments[0].CapabilitySnapshotID != snapshot.ID {
		t.Fatalf("snapshot linkage was not persisted: %#v", deployments)
	}
}

func TestRunModelStepPersistsReportWhenModelExceedsOutputBudget(t *testing.T) {
	ctx := context.Background()
	provider := &recordingPermissionProvider{responses: []contracts.ChatResponse{{Text: "too long", Usage: contracts.TokenUsage{OutputTokens: 2049}}}}
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "inspect", Goal: "inspect", AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "agent", Type: contracts.AcceptanceEvidenceExists, Description: "report", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "local", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RunModelStep(ctx, RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrBudgetExceeded {
		t.Fatalf("expected a budget error, got %v", err)
	}
	artifacts, err := service.ListTaskArtifacts(ctx, created.ID)
	if err != nil || len(artifacts) != 1 || artifacts[0].Kind != "AGENT_REPORT" {
		t.Fatalf("failed turn did not retain its diagnostic report: %#v, %v", artifacts, err)
	}
}

func TestApprovedWriteFilePersistsIntentAndRecovers(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	target := filepath.Join(root, "note.txt")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "write", Goal: "write a note"})
	if err != nil {
		t.Fatal(err)
	}
	step := contracts.StepSpec{Version: contracts.SchemaVersion, StepID: "write-step", Title: "Write", Goal: "write the approved note", AllowedTools: []string{"write_file"}, WorkspaceScopes: []string{"."}, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "file", Type: contracts.AcceptanceFileExists, Description: "note exists", Spec: map[string]interface{}{"path": "note.txt"}}}, Risk: contracts.RiskWrite, Budget: contracts.StepBudget{MaxAttempts: 1, MaxIterations: 1, MaxDurationMS: 1000}, ExecutionMode: "CONTROLLED"}
	planVersion := contracts.PlanVersion{Version: contracts.SchemaVersion, PlanID: "write-plan", TaskID: created.ID, Revision: 2, ParentPlanID: "initial", Reason: "TEST", Summary: "write", Steps: []contracts.StepSpec{step}, CreatedByAgent: "test", CreatedAt: time.Now().UTC()}
	event, err := service.newEvent(ctx, created.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": planVersion.PlanID, "revision": planVersion.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.store.CreatePlan(ctx, planVersion, event); err != nil {
		t.Fatal(err)
	}
	if err = service.transitionTask(ctx, created.ID, contracts.TaskReady, contracts.TaskRunning, "TASK_STATUS_CHANGED"); err != nil {
		t.Fatal(err)
	}
	if err = service.transitionStep(ctx, created.ID, step.StepID, contracts.StepPending, contracts.StepReady, "STEP_STATUS_CHANGED"); err != nil {
		t.Fatal(err)
	}
	if err = service.transitionStep(ctx, created.ID, step.StepID, contracts.StepReady, contracts.StepRunning, "STEP_STATUS_CHANGED"); err != nil {
		t.Fatal(err)
	}
	input := WriteFileInput{TaskID: created.ID, StepID: step.StepID, Path: "note.txt", Content: "new", ExpectedContentHash: contentHash([]byte("old"))}
	ticket, err := service.ApproveWriteFile(ctx, input, time.Now().Add(time.Minute))
	if err != nil || ticket.ID == "" {
		t.Fatal(ticket, err)
	}
	result, err := service.WriteFile(ctx, input)
	if err != nil || result.Status != "SUCCEEDED" || result.Data["recovered"] != false {
		t.Fatal(result, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new" {
		t.Fatal(string(data), err)
	}
	intent, err := writeIntent(created.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.store.GetToolIntent(ctx, intent.IdempotencyKey)
	if err != nil || record.Status != "SUCCEEDED" || result.ToolCallID != record.ID {
		t.Fatal(record, err)
	}
	retry, err := service.WriteFile(ctx, input)
	if err != nil || retry.Status != "SUCCEEDED" || retry.Data["recovered"] != true {
		t.Fatal(retry, err)
	}
	snapshot, err := service.GetTaskSnapshot(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundApproval, intentEvents := false, 0
	for _, taskEvent := range snapshot.Events {
		foundApproval = foundApproval || taskEvent.EventType == "TOOL_APPROVED"
		if taskEvent.EventType == "TOOL_INTENT_RECORDED" {
			intentEvents++
		}
	}
	if !foundApproval || intentEvents != 1 {
		t.Fatalf("unexpected approval audit events: approved=%t intents=%d", foundApproval, intentEvents)
	}
}

func TestRecoverRunningTaskPausesFromCheckpoint(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "recover", Goal: "recover safely"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.transitionTask(ctx, created.ID, contracts.TaskReady, contracts.TaskRunning, "TASK_STATUS_CHANGED"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.CheckpointTask(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	before, err := service.store.GetLatestCheckpoint(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RecoverRunningTask(ctx, created.ID)
	if err != nil || snapshot.Task.Status != contracts.TaskPaused {
		t.Fatal(snapshot, err)
	}
	after, err := service.store.GetLatestCheckpoint(ctx, created.ID)
	if err != nil || after.Sequence <= before.Sequence {
		t.Fatal(after, err)
	}
	last := snapshot.Events[len(snapshot.Events)-1]
	if last.EventType != "TASK_RECOVERY_PAUSED" {
		t.Fatal(last)
	}
}

func TestReplanPausedTaskPersistsLocalRevision(t *testing.T) {
	ctx := context.Background()
	response := `{"summary":"replan","steps":[{"version":1,"step_id":"new-inspect","title":"Inspect","goal":"Read files","allowed_tools":["list_files"],"workspace_scopes":["."],"acceptance_criteria":[{"id":"evidence","type":"EVIDENCE_EXISTS","description":"report","spec":{}}],"risk":"READ","budget":{"max_attempts":1,"max_iterations":2,"max_duration_ms":1000,"max_input_tokens":10,"max_output_tokens":10}}]}`
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: response}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, initial, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "replan", Goal: "replan safely"})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.transitionTask(ctx, created.ID, contracts.TaskReady, contracts.TaskRunning, "TASK_STATUS_CHANGED"); err != nil {
		t.Fatal(err)
	}
	if _, err = service.RecoverRunningTask(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	replanned, err := service.ReplanTask(ctx, created.ID, deployment.ID, "interrupted before execution")
	if err != nil || replanned.Revision != 2 || replanned.ParentPlanID != initial.PlanID || replanned.Reason != "LOCAL_REPLAN" {
		t.Fatal(replanned, err)
	}
	snapshot, err := service.GetTaskSnapshot(ctx, created.ID)
	if err != nil || snapshot.Task.Status != contracts.TaskReady || snapshot.Plan.PlanID != replanned.PlanID {
		t.Fatal(snapshot, err)
	}
	found := false
	for _, event := range snapshot.Events {
		found = found || event.EventType == "PLAN_REPLANNED"
	}
	if !found {
		t.Fatal("local replan event was not persisted")
	}
}

func TestRunModelStepPersistsAgentReportAndVerifiesTask(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: "inspected workspace"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "agent", Goal: "inspect", AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "agent-report", Type: contracts.AcceptanceEvidenceExists, Description: "agent report", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	step := contracts.StepSpec{Version: contracts.SchemaVersion, StepID: "agent-step", Title: "Inspect", Goal: "inspect workspace", AllowedTools: []string{"list_files"}, WorkspaceScopes: []string{"."}, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "agent-report", Type: contracts.AcceptanceEvidenceExists, Description: "agent report", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}, Risk: contracts.RiskRead, Budget: contracts.StepBudget{MaxAttempts: 1, MaxIterations: 1, MaxDurationMS: 1000, MaxInputTokens: 1000, MaxOutputTokens: 1000}, ExecutionMode: "AGENT"}
	planVersion := contracts.PlanVersion{Version: contracts.SchemaVersion, PlanID: "agent-plan", TaskID: created.ID, Revision: 2, ParentPlanID: "initial", Reason: "TEST", Summary: "agent", Steps: []contracts.StepSpec{step}, CreatedByAgent: "test", CreatedAt: time.Now().UTC()}
	event, err := service.newEvent(ctx, created.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": planVersion.PlanID, "revision": planVersion.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.store.CreatePlan(ctx, planVersion, event); err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunModelStep(ctx, RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || snapshot.Task.Status != contracts.TaskCompleted || len(snapshot.Steps) != 1 || len(snapshot.Steps[0].EvidenceIDs) != 1 {
		t.Fatal(snapshot, err)
	}
	found := false
	for _, event := range snapshot.Events {
		found = found || event.EventType == "TASK_STATUS_CHANGED"
	}
	if !found {
		t.Fatal("model execution state changes were not audited")
	}
}

func TestRunModelPlanExecutesEverySequentialStep(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: "completed read-only step"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "run plan", Goal: "inspect", AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "agent_report", Type: contracts.AcceptanceEvidenceExists, Description: "report", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.store.GetLatestPlan(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	first := current.Steps[0]
	first.StepID = task.NewID("stp")
	second := first
	second.StepID = task.NewID("stp")
	second.Title = "Second"
	second.Dependencies = []string{first.StepID}
	current.Steps = []contracts.StepSpec{first, second}
	current.PlanID = task.NewID("pln")
	current.Revision++
	event, err := service.newEvent(ctx, created.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": current.PlanID})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.store.CreatePlan(ctx, current, event); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunModelPlan(ctx, RunModelStepInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || snapshot.Task.Status != contracts.TaskCompleted || len(snapshot.Steps) != 2 || snapshot.Steps[0].Status != contracts.StepCompleted || snapshot.Steps[1].Status != contracts.StepCompleted {
		t.Fatal(snapshot, err)
	}
}

func TestAssignReadOnlyAgentPersistsOneLayerCapabilitySnapshot(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, planVersion, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "delegate", Goal: "inspect", AllowSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := service.AssignReadOnlyAgent(ctx, AssignAgentInput{TaskID: created.ID, StepID: planVersion.Steps[0].StepID, DeploymentID: deployment.ID, Role: "RECON"})
	if err != nil || agent.ID == "" || agent.Depth != 1 || agent.Status != contracts.AgentPending || len(agent.AllowedTools) == 0 {
		t.Fatal(agent, err)
	}
	assigned, err := service.ListAgentAssignments(ctx, created.ID)
	if err != nil || len(assigned) != 1 || assigned[0].ID != agent.ID || assigned[0].Depth != 1 || len(assigned[0].AllowedTools) != len(agent.AllowedTools) {
		t.Fatal(assigned, err)
	}
	snapshot, err := service.GetTaskSnapshot(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range snapshot.Events {
		found = found || event.EventType == "AGENT_ASSIGNED"
	}
	if !found {
		t.Fatal("agent assignment event was not persisted")
	}
	created.Spec.AllowSubagents = false
	if _, err = service.AssignReadOnlyAgent(ctx, AssignAgentInput{TaskID: created.ID, StepID: planVersion.Steps[0].StepID, DeploymentID: deployment.ID, Role: "RECON"}); err == nil {
		t.Fatal("second active assignment should be rejected")
	}
}

func TestRunAssignedAgentPersistsHandoffAndStatus(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: "inspected"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, planVersion, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "delegate", Goal: "inspect", AllowSubagents: true, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "agent-report", Type: contracts.AcceptanceEvidenceExists, Description: "report", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := service.AssignReadOnlyAgent(ctx, AssignAgentInput{TaskID: created.ID, StepID: planVersion.Steps[0].StepID, DeploymentID: deployment.ID, Role: "RECON"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunAssignedAgent(ctx, agent.ID)
	if err != nil || snapshot.Task.Status != contracts.TaskCompleted {
		t.Fatal(snapshot, err)
	}
	assigned, err := service.ListAgentAssignments(ctx, created.ID)
	if err != nil || len(assigned) != 1 || assigned[0].Status != contracts.AgentSucceeded {
		t.Fatal(assigned, err)
	}
	starts, finishes := 0, 0
	for _, event := range snapshot.Events {
		if event.EventType == "AGENT_STATUS_CHANGED" && event.Payload["to"] == string(contracts.AgentRunning) {
			starts++
		}
		if event.EventType == "AGENT_STATUS_CHANGED" && event.Payload["to"] == string(contracts.AgentSucceeded) {
			finishes++
		}
	}
	artifacts, err := service.ListTaskArtifacts(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundHandoff := false
	for _, artifact := range artifacts {
		foundHandoff = foundHandoff || artifact.Kind == "AGENT_HANDOFF"
	}
	if starts != 1 || finishes != 1 || !foundHandoff {
		t.Fatalf("expected agent lifecycle and report artifacts: starts=%d finishes=%d snapshot=%#v", starts, finishes, snapshot)
	}
}

func TestRunCoordinatorCycleCreatesAndRunsOneAssignment(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: "inspected"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "coordinate", Goal: "inspect", AllowSubagents: true, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "agent-report", Type: contracts.AcceptanceEvidenceExists, Description: "report", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RunCoordinatorCycle(ctx, CoordinatorCycleInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || snapshot.Task.Status != contracts.TaskCompleted {
		t.Fatal(snapshot, err)
	}
	assignments, err := service.ListAgentAssignments(ctx, created.ID)
	if err != nil || len(assignments) != 1 || assignments[0].Role != "RECON" || assignments[0].Status != contracts.AgentSucceeded {
		t.Fatal(assignments, err)
	}
}

func TestCoordinatorRunsDependentReadOnlyStepsAcrossCycles(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: "inspected"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "two steps", Goal: "inspect", AllowSubagents: true, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "report", Type: contracts.AcceptanceEvidenceExists, Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	criterion := []contracts.AcceptanceCriterion{{ID: "report", Type: contracts.AcceptanceEvidenceExists, Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}
	first := contracts.StepSpec{Version: contracts.SchemaVersion, StepID: "first", Title: "First", Goal: "first", AllowedTools: []string{"list_files"}, WorkspaceScopes: []string{"."}, AcceptanceCriteria: criterion, Risk: contracts.RiskRead, Budget: contracts.StepBudget{MaxAttempts: 1, MaxIterations: 1, MaxDurationMS: 1000, MaxInputTokens: 1000, MaxOutputTokens: 1000}, PreferredRole: "RECON"}
	second := first
	second.StepID, second.Title, second.Dependencies = "second", "Second", []string{"first"}
	planVersion := contracts.PlanVersion{Version: contracts.SchemaVersion, PlanID: "two-step-plan", TaskID: created.ID, Revision: 2, ParentPlanID: "initial", Reason: "TEST", Summary: "two", Steps: []contracts.StepSpec{first, second}, CreatedAt: time.Now().UTC()}
	event, err := service.newEvent(ctx, created.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": planVersion.PlanID, "revision": planVersion.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.store.CreatePlan(ctx, planVersion, event); err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	firstSnapshot, err := service.RunCoordinatorCycle(ctx, CoordinatorCycleInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || firstSnapshot.Task.Status != contracts.TaskRunning {
		t.Fatal(firstSnapshot, err)
	}
	finalSnapshot, err := service.RunCoordinatorCycle(ctx, CoordinatorCycleInput{TaskID: created.ID, DeploymentID: deployment.ID})
	if err != nil || finalSnapshot.Task.Status != contracts.TaskCompleted {
		t.Fatal(finalSnapshot, err)
	}
}

func TestRecoverRunningAgentAssignmentsFailsAgentAndPausesTask(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, planVersion, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "recover agent", Goal: "inspect", AllowSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	agent, err := service.AssignReadOnlyAgent(ctx, AssignAgentInput{TaskID: created.ID, StepID: planVersion.Steps[0].StepID, DeploymentID: deployment.ID, Role: "RECON"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.transitionTask(ctx, created.ID, contracts.TaskReady, contracts.TaskRunning, "TASK_STATUS_CHANGED"); err != nil {
		t.Fatal(err)
	}
	if err = service.transitionAgent(ctx, agent, contracts.AgentPending, contracts.AgentRunning, "AGENT_STATUS_CHANGED"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.RecoverRunningAgentAssignments(ctx, created.ID)
	if err != nil || snapshot.Task.Status != contracts.TaskPaused {
		t.Fatal(snapshot, err)
	}
	assignments, err := service.ListAgentAssignments(ctx, created.ID)
	if err != nil || len(assignments) != 1 || assignments[0].Status != contracts.AgentFailed {
		t.Fatal(assignments, err)
	}
	found := false
	for _, event := range snapshot.Events {
		found = found || event.EventType == "AGENT_RECOVERY_FAILED"
	}
	if !found {
		t.Fatal("agent recovery event was not persisted")
	}
}

func contentHash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func TestGeneratePlanPersistsModelRevision(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	response := `{"summary":"model plan","reason":"MODEL","steps":[{"version":1,"step_id":"inspect","title":"Inspect","goal":"Read files","allowed_tools":["list_files"],"workspace_scopes":["."],"acceptance_criteria":[{"id":"evidence","type":"EVIDENCE_EXISTS","description":"report","spec":{}}],"risk":"READ","budget":{"max_attempts":1,"max_iterations":2,"max_duration_ms":1000,"max_input_tokens":10,"max_output_tokens":10}}]}`
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: response}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(context.Background(), "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(context.Background(), CreateTaskInput{WorkspaceID: workspace.ID, Title: "demo", Goal: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := service.GeneratePlan(context.Background(), created.ID, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if generated.Revision != 2 || generated.ParentPlanID == "" {
		t.Fatalf("unexpected plan revision: %#v", generated)
	}
	snapshot, err := service.GetTaskSnapshot(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan.PlanID != generated.PlanID || len(snapshot.Events) < 5 {
		t.Fatalf("plan was not persisted with event: %#v", snapshot)
	}
}
