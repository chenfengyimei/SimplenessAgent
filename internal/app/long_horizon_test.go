package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

type longHorizonProvider struct {
	responses []contracts.ChatResponse
	requests  []contracts.ChatRequest
	window    int
}

func TestLongHorizonRunsTwentyStepsAndRecoversAtCycleBoundary(t *testing.T) {
	ctx := context.Background()
	provider := &longHorizonProvider{}
	for stageIndex := 0; stageIndex < 5; stageIndex++ {
		steps := make([]string, 0, 4)
		for stepIndex := 0; stepIndex < 4; stepIndex++ {
			steps = append(steps, fmt.Sprintf(`{"ref":"s%d","title":"Step %d","goal":"Record bounded evidence %d","tool_intents":["list_files"],"acceptance_intent":"Agent report exists"}`, stepIndex, stepIndex, stepIndex))
		}
		provider.responses = append(provider.responses, contracts.ChatResponse{Text: `{"summary":"bounded segment","terminal_segment":true,"steps":[` + strings.Join(steps, ",") + `]}`})
		for stepIndex := 0; stepIndex < 4; stepIndex++ {
			provider.responses = append(provider.responses, contracts.ChatResponse{Text: "bounded evidence"})
		}
		provider.responses = append(provider.responses, contracts.ChatResponse{Text: `{"summary":"persisted evidence reviewed","gate_appears_met":true,"evidence_refs":[],"risks":[]}`})
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	workspaceRoot := t.TempDir()
	openService := func() *Service {
		service, err := Open(ctx, Config{DataDir: dataDir, ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
		if err != nil {
			t.Fatal(err)
		}
		return service
	}
	service := openService()
	workspaceItem, err := service.CreateWorkspace(ctx, "twenty", workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "small", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "small", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateLongHorizonTask(ctx, CreateLongHorizonTaskInput{DeploymentID: deployment.ID, CreateTaskInput: CreateTaskInput{WorkspaceID: workspaceItem.ID, Title: "twenty steps", Goal: "exercise durable cycles", PermissionMode: contracts.PermissionModePlan, StageCheckpointPolicy: contracts.StageCheckpointNone}})
	if err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 9; cycle++ {
		if _, err = service.AdvanceLongHorizonTask(ctx, created.ID); err != nil {
			t.Fatalf("pre-restart cycle %d: %v", cycle, err)
		}
	}
	before, err := service.GetLongHorizonStatus(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Close(); err != nil {
		t.Fatal(err)
	}
	service = openService()
	defer service.Close()
	after, err := service.GetLongHorizonStatus(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.StepsPlanned != after.StepsPlanned || before.StepsCompleted != after.StepsCompleted || before.CurrentStageIndex != after.CurrentStageIndex || before.LastProcessedPlanID != after.LastProcessedPlanID {
		t.Fatalf("cycle-boundary recovery changed durable state: before=%#v after=%#v", before, after)
	}
	for cycle := 0; cycle < 60; cycle++ {
		result, advanceErr := service.AdvanceLongHorizonTask(ctx, created.ID)
		if advanceErr != nil {
			t.Fatalf("post-restart cycle %d: %v", cycle, advanceErr)
		}
		if result.Status == contracts.HorizonCompleted {
			break
		}
	}
	finalState, err := service.GetLongHorizonStatus(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finalState.Status != contracts.HorizonCompleted || finalState.StepsPlanned != 20 || finalState.StepsCompleted != 20 || finalState.CurrentStageIndex != 5 {
		t.Fatalf("twenty-step horizon did not complete deterministically: %#v", finalState)
	}
	snapshot, err := service.GetTaskSnapshot(ctx, created.ID)
	if err != nil || snapshot.Task.Status != contracts.TaskCompleted {
		t.Fatalf("global deterministic acceptance did not complete the task: %#v %v", snapshot.Task, err)
	}
}

func (p *longHorizonProvider) Chat(_ context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return contracts.ChatResponse{Text: "done"}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}
func (p *longHorizonProvider) ChatStream(context.Context, contracts.ChatRequest, contracts.StreamSink) error {
	return nil
}
func (p *longHorizonProvider) HealthCheck(context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true}
}
func (p *longHorizonProvider) ProbeCapabilities(context.Context) contracts.CapabilitySnapshot {
	window := p.window
	if window == 0 {
		window = 8192
	}
	return contracts.CapabilitySnapshot{Version: contracts.SchemaVersion, SupportsTools: true, ReliableContextTokens: window, ProbedAt: time.Now().UTC()}
}

func TestCreateLongHorizonFailsClosedForVerified4KDeployment(t *testing.T) {
	ctx := context.Background()
	provider := &longHorizonProvider{window: 4096}
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspaceItem, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "4k", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "small", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = service.ProbeDeployment(ctx, deployment.ID); err != nil {
		t.Fatal(err)
	}
	_, _, err = service.CreateLongHorizonTask(ctx, CreateLongHorizonTaskInput{DeploymentID: deployment.ID, CreateTaskInput: CreateTaskInput{WorkspaceID: workspaceItem.ID, Title: "too small", Goal: "repair project"}})
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrContextOverflow {
		t.Fatalf("expected 4K deployment to fail closed, got %v", err)
	}
}

func TestLongHorizonPlannerFormatFailureReturnsDurablePausedCycle(t *testing.T) {
	ctx := context.Background()
	provider := &longHorizonProvider{responses: []contracts.ChatResponse{
		{Text: `{"summary":7,"terminal_segment":true,"steps":[]}`},
		{Text: `{"summary":7,"terminal_segment":true,"steps":[]}`},
	}}
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspaceItem, err := service.CreateWorkspace(ctx, "paused", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "small", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "small", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateLongHorizonTask(ctx, CreateLongHorizonTaskInput{DeploymentID: deployment.ID, CreateTaskInput: CreateTaskInput{WorkspaceID: workspaceItem.ID, Title: "planner format", Goal: "build a project", PermissionMode: contracts.PermissionModePlan, StageCheckpointPolicy: contracts.StageCheckpointNone}})
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := service.AdvanceLongHorizonTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("a persisted planner pause must not surface as a service error: %v", err)
	}
	if cycle.Status != contracts.HorizonPaused || cycle.Action != "FAILED_PAUSED" || !strings.Contains(cycle.CheckpointReason, "summary must be a string") {
		t.Fatalf("planner failure was not returned as a diagnostic paused cycle: %#v", cycle)
	}
	state, err := service.GetLongHorizonStatus(ctx, created.ID)
	if err != nil || state.LatestFailureArtifactID == "" || state.ReplansUsed != 1 {
		t.Fatalf("planner failure was not durably checkpointed: state=%#v err=%v", state, err)
	}
	snapshot, err := service.GetTaskSnapshot(ctx, created.ID)
	if err != nil || snapshot.Task.Status != contracts.TaskPaused {
		t.Fatalf("planner failure did not pause the task itself: task=%#v err=%v", snapshot.Task, err)
	}
}

func TestLongHorizonResumeReplansAfterFailedExecutorStep(t *testing.T) {
	ctx := context.Background()
	tooMany := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{
		{ID: "first", Name: "list_files", ArgumentsJSON: `{}`},
		{ID: "second", Name: "list_files", ArgumentsJSON: `{}`},
	}}
	provider := &longHorizonProvider{responses: []contracts.ChatResponse{
		{Text: `{"summary":"discover","terminal_segment":true,"steps":[{"ref":"first","title":"First","goal":"Inspect the root","tool_intents":["list_files"],"acceptance_intent":"Root is inspected"},{"ref":"second","title":"Second","goal":"Inspect dependencies","dependencies":["first"],"tool_intents":["list_files"],"acceptance_intent":"Dependencies are identified"}]}`},
		{Text: "First step evidence."},
		tooMany,
		tooMany,
		{Text: `{"summary":"replanned discover","terminal_segment":true,"steps":[{"ref":"recover","title":"Recover discovery","goal":"Inspect the remaining project surface","tool_intents":["list_files"],"acceptance_intent":"Remaining files are identified"}]}`},
	}}
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspaceItem, err := service.CreateWorkspace(ctx, "resume", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "small", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "small", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateLongHorizonTask(ctx, CreateLongHorizonTaskInput{DeploymentID: deployment.ID, CreateTaskInput: CreateTaskInput{WorkspaceID: workspaceItem.ID, Title: "resume", Goal: "inspect project", PermissionMode: contracts.PermissionModePlan, StageCheckpointPolicy: contracts.StageCheckpointNone}})
	if err != nil {
		t.Fatal(err)
	}
	for cycleIndex := 0; cycleIndex < 2; cycleIndex++ {
		if _, err = service.AdvanceLongHorizonTask(ctx, created.ID); err != nil {
			t.Fatalf("pre-failure cycle %d: %v", cycleIndex, err)
		}
	}
	failed, err := service.AdvanceLongHorizonTask(ctx, created.ID)
	if err != nil || failed.Status != contracts.HorizonPaused || failed.Action != "FAILED_PAUSED" {
		t.Fatalf("executor failure was not paused: cycle=%#v err=%v", failed, err)
	}
	pausedState, err := service.GetLongHorizonStatus(ctx, created.ID)
	if err != nil || pausedState.ReplansUsed != 1 || pausedState.LastProcessedPlanID == "" {
		t.Fatalf("failed segment was not checkpointed once: state=%#v err=%v", pausedState, err)
	}
	failedPlanID := pausedState.LastProcessedPlanID
	// Simulate the checkpoint shape written by the previous release so this test
	// also proves the user's already-paused task can be resumed after upgrading.
	pausedState.LastProcessedPlanID = ""
	if err = service.saveHorizonState(ctx, &pausedState, "TEST_LEGACY_FAILURE_CHECKPOINT", nil); err != nil {
		t.Fatal(err)
	}
	resumed, err := service.ResumeLongHorizonTask(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.LastProcessedPlanID != failedPlanID {
		t.Fatalf("legacy paused checkpoint did not abandon its failed plan: state=%#v", resumed)
	}
	replanned, err := service.AdvanceLongHorizonTask(ctx, created.ID)
	if err != nil || replanned.Action != "SEGMENT_PLANNED" || replanned.Status != contracts.HorizonActive {
		t.Fatalf("resume did not immediately create a replacement segment: cycle=%#v err=%v", replanned, err)
	}
	resumedState, err := service.GetLongHorizonStatus(ctx, created.ID)
	if err != nil || resumedState.ReplansUsed != 1 || resumedState.StepsPlanned != 3 {
		t.Fatalf("resume charged the failure twice or lost progress: state=%#v err=%v", resumedState, err)
	}
}

func TestLongHorizonAdvancesSegmentsAndWaitsAfterDesign(t *testing.T) {
	ctx := context.Background()
	provider := &longHorizonProvider{responses: []contracts.ChatResponse{
		{Text: `{"summary":"discover","terminal_segment":true,"steps":[{"ref":"scan","title":"Scan","goal":"Locate relevant files","tool_intents":["list_files"],"acceptance_intent":"Relevant files are identified"}]}`},
		{Text: "Discovery evidence recorded."},
		{Text: `{"summary":"discovery evidence is present","gate_appears_met":true,"evidence_refs":[],"risks":[]}`},
		{Text: `{"summary":"design","terminal_segment":true,"steps":[{"ref":"design","title":"Design","goal":"Define the bounded implementation","tool_intents":["read_file"],"acceptance_intent":"Implementation route is explicit"}]}`},
		{Text: "Design evidence recorded."},
		{Text: `{"summary":"design evidence is present","gate_appears_met":true,"evidence_refs":[],"risks":[]}`},
	}}
	dataDir := filepath.Join(t.TempDir(), "data")
	service, err := Open(ctx, Config{DataDir: dataDir, ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) { return provider, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspaceItem, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(ctx, contracts.Deployment{Name: "small", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "small", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	created, state, err := service.CreateLongHorizonTask(ctx, CreateLongHorizonTaskInput{DeploymentID: deployment.ID, CreateTaskInput: CreateTaskInput{WorkspaceID: workspaceItem.ID, Title: "long fix", Goal: "repair the project", PermissionMode: contracts.PermissionModeEdit, AllowWriteProposals: true}})
	if err != nil {
		t.Fatal(err)
	}
	if created.Spec.Budget.MaxSteps != 20 || created.Spec.Budget.MaxReplans != 4 || len(state.Plan.Stages) != 5 {
		t.Fatalf("long-horizon defaults were not applied: %#v %#v", created.Spec, state)
	}
	actions := []string{}
	for index := 0; index < 6; index++ {
		result, advanceErr := service.AdvanceLongHorizonTask(ctx, created.ID)
		if advanceErr != nil {
			t.Fatalf("cycle %d failed: %v", index, advanceErr)
		}
		actions = append(actions, result.Action)
	}
	latest, err := service.GetLongHorizonStatus(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !latest.AwaitingCheckpoint || latest.Status != contracts.HorizonWaiting || latest.CurrentStageIndex != 2 {
		t.Fatalf("design checkpoint was not persisted: actions=%v state=%#v", actions, latest)
	}
	snapshot, _ := service.GetTaskSnapshot(ctx, created.ID)
	if snapshot.Task.Status != contracts.TaskWaitingUser {
		t.Fatalf("task did not enter WAITING_USER: %#v", snapshot.Task)
	}
	resumed, err := service.ResumeLongHorizonTask(ctx, created.ID)
	if err != nil || resumed.Status != contracts.HorizonActive || resumed.AwaitingCheckpoint {
		t.Fatalf("resume failed: %#v %v", resumed, err)
	}
	artifacts, err := service.ListTaskArtifacts(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, artifact := range artifacts {
		kinds[artifact.Kind] = true
	}
	for _, kind := range []string{"HORIZON_PLAN", "PROGRESS_LEDGER", "STAGE_SUMMARY", "AGENT_REPORT", "CONTEXT_MANIFEST"} {
		if !kinds[kind] {
			t.Fatalf("missing %s artifact: %#v", kind, artifacts)
		}
	}
}
