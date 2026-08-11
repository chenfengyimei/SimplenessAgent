// Package app composes domain services into the single local Agent Core used by
// both the CLI and eventual Wails bindings.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/artifact"
	"github.com/xm/simplenessagent/internal/eventstore"
	"github.com/xm/simplenessagent/internal/plan"
	"github.com/xm/simplenessagent/internal/planner"
	"github.com/xm/simplenessagent/internal/policy"
	"github.com/xm/simplenessagent/internal/storage"
	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/internal/tool"
	"github.com/xm/simplenessagent/internal/verifier"
	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

type Service struct {
	store           *storage.Store
	artifactStore   *artifact.Store
	dataDir         string
	resolveProvider func(contracts.Deployment) (contracts.ChatProvider, error)
}

type Config struct {
	DataDir         string
	ResolveProvider func(contracts.Deployment) (contracts.ChatProvider, error)
}

func Open(ctx context.Context, config Config) (*Service, error) {
	if strings.TrimSpace(config.DataDir) == "" {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "data directory is required")
	}
	if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
		return nil, err
	}
	store, err := storage.Open(ctx, filepath.Join(config.DataDir, "simpleness.db"))
	if err != nil {
		return nil, err
	}
	artifactStore, err := artifact.NewStore(filepath.Join(config.DataDir, "artifacts"))
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return &Service{store: store, artifactStore: artifactStore, dataDir: config.DataDir, resolveProvider: config.ResolveProvider}, nil
}

func (s *Service) Close() error    { return s.store.Close() }
func (s *Service) DataDir() string { return s.dataDir }

func (s *Service) CreateWorkspace(ctx context.Context, name, path string) (contracts.Workspace, error) {
	root, err := workspace.NormalizeRoot(path)
	if err != nil {
		return contracts.Workspace{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(root)
	}
	now := time.Now().UTC()
	item := contracts.Workspace{ID: task.NewID("ws"), Version: contracts.SchemaVersion, Name: name, RootPath: root, CreatedAt: now, UpdatedAt: now}
	if err = s.store.CreateWorkspace(ctx, item); err != nil {
		return contracts.Workspace{}, err
	}
	return item, nil
}
func (s *Service) ListWorkspaces(ctx context.Context) ([]contracts.Workspace, error) {
	return s.store.ListWorkspaces(ctx)
}
func (s *Service) ListTasks(ctx context.Context, workspaceID string) ([]contracts.Task, error) {
	return s.store.ListTasks(ctx, workspaceID)
}

func (s *Service) CreateDeployment(ctx context.Context, item contracts.Deployment) (contracts.Deployment, error) {
	if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.ProviderType) == "" || strings.TrimSpace(item.Endpoint) == "" || strings.TrimSpace(item.Model) == "" {
		return contracts.Deployment{}, contracts.NewError(contracts.ErrInvalidInput, "deployment name, provider type, endpoint and model are required")
	}
	now := time.Now().UTC()
	item.ID = task.NewID("dep")
	item.Version = contracts.SchemaVersion
	item.CreatedAt = now
	item.UpdatedAt = now
	if err := s.store.CreateDeployment(ctx, item); err != nil {
		return contracts.Deployment{}, err
	}
	return item, nil
}
func (s *Service) ListDeployments(ctx context.Context) ([]contracts.Deployment, error) {
	return s.store.ListDeployments(ctx)
}
func (s *Service) ProbeDeployment(ctx context.Context, deploymentID string) (contracts.HealthStatus, contracts.CapabilitySnapshot, error) {
	if s.resolveProvider == nil {
		return contracts.HealthStatus{}, contracts.CapabilitySnapshot{}, contracts.NewError(contracts.ErrCapabilityUnsupported, "no provider resolver is configured")
	}
	deployment, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return contracts.HealthStatus{}, contracts.CapabilitySnapshot{}, err
	}
	provider, err := s.resolveProvider(deployment)
	if err != nil {
		return contracts.HealthStatus{}, contracts.CapabilitySnapshot{}, err
	}
	health := provider.HealthCheck(ctx)
	snapshot := provider.ProbeCapabilities(ctx)
	snapshot.ID = task.NewID("cap")
	snapshot.DeploymentID = deployment.ID
	snapshot.Version = contracts.SchemaVersion
	if err = s.store.SaveCapabilitySnapshot(ctx, snapshot); err != nil {
		return contracts.HealthStatus{}, contracts.CapabilitySnapshot{}, err
	}
	return health, snapshot, nil
}

// GeneratePlan creates and atomically persists a locally-validated revision.
// It does not alter task state, so callers may review the plan before running.
func (s *Service) GeneratePlan(ctx context.Context, taskID, deploymentID string) (contracts.PlanVersion, error) {
	if s.resolveProvider == nil {
		return contracts.PlanVersion{}, contracts.NewError(contracts.ErrCapabilityUnsupported, "no provider resolver is configured")
	}
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	deployment, err := s.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	if !deployment.Enabled {
		return contracts.PlanVersion{}, contracts.NewError(contracts.ErrPermissionDenied, "deployment is disabled")
	}
	provider, err := s.resolveProvider(deployment)
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	workspaceItem, err := s.store.GetWorkspace(ctx, item.WorkspaceID)
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	registry := tool.NewRegistry()
	if err = tool.RegisterReadOnly(registry, workspaceItem.RootPath); err != nil {
		return contracts.PlanVersion{}, err
	}
	previous, err := s.store.GetLatestPlan(ctx, item.ID)
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	plannerService, err := planner.New(provider)
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	candidate, err := plannerService.Create(ctx, planner.Input{DeploymentID: deployment.ID, Task: item, AvailableTools: registry.Definitions(), Revision: previous.Revision + 1, ParentPlanID: previous.PlanID})
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	event, err := s.newEvent(ctx, item.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": candidate.PlanID, "revision": candidate.Revision, "parent_plan_id": candidate.ParentPlanID, "source": "MODEL"})
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	if err = s.store.CreatePlan(ctx, candidate, event); err != nil {
		return contracts.PlanVersion{}, err
	}
	return candidate, nil
}

type CreateTaskInput struct {
	WorkspaceID, Title, Goal string
	Constraints              []contracts.Constraint
	AcceptanceCriteria       []contracts.AcceptanceCriterion
	Budget                   contracts.TaskBudget
}

// WriteFileInput is the complete, parameter-bound request for a file write.
// The expected hash prevents a stale plan or approval from overwriting newer
// workspace content.
type WriteFileInput struct {
	TaskID, StepID, Path, Content, ExpectedContentHash string
}

// ApproveWriteFile creates a single-use approval ticket for exactly one file
// write. It may be called before a step starts, but WriteFile rechecks that the
// step is currently running before it permits any side effect.
func (s *Service) ApproveWriteFile(ctx context.Context, input WriteFileInput, expiresAt time.Time) (contracts.ApprovalTicket, error) {
	if err := validateWriteInput(input); err != nil {
		return contracts.ApprovalTicket{}, err
	}
	item, step, workspaceItem, err := s.writeContext(ctx, input.TaskID, input.StepID, false)
	if err != nil {
		return contracts.ApprovalTicket{}, err
	}
	if err = validateWriteScope(workspaceItem.RootPath, step.WorkspaceScopes, input.Path); err != nil {
		return contracts.ApprovalTicket{}, err
	}
	intent, err := writeIntent(item.ID, input)
	if err != nil {
		return contracts.ApprovalTicket{}, err
	}
	ticket := contracts.ApprovalTicket{ID: task.NewID("apr"), TaskID: item.ID, StepID: input.StepID, ToolName: intent.ToolName, ArgumentsHash: intent.ArgumentsHash, Decision: "APPROVED", ExpiresAt: expiresAt, UsesRemaining: 1, CreatedAt: time.Now().UTC()}
	event, err := s.newEvent(ctx, item.ID, "TOOL_APPROVED", map[string]interface{}{"approval_id": ticket.ID, "step_id": ticket.StepID, "tool_name": ticket.ToolName, "arguments_hash": ticket.ArgumentsHash, "expires_at": ticket.ExpiresAt})
	if err != nil {
		return contracts.ApprovalTicket{}, err
	}
	if err = s.store.CreateApprovalWithEvent(ctx, ticket, event); err != nil {
		return contracts.ApprovalTicket{}, err
	}
	return ticket, nil
}

// WriteFile records a durable intent before executing a user-approved,
// workspace-scoped atomic file replacement.
func (s *Service) WriteFile(ctx context.Context, input WriteFileInput) (contracts.ToolResult, error) {
	if err := validateWriteInput(input); err != nil {
		return contracts.ToolResult{}, err
	}
	item, step, workspaceItem, err := s.writeContext(ctx, input.TaskID, input.StepID, true)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if err = validateWriteScope(workspaceItem.RootPath, step.WorkspaceScopes, input.Path); err != nil {
		return contracts.ToolResult{}, err
	}
	intent, err := writeIntent(item.ID, input)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	event, err := s.newEvent(ctx, item.ID, "TOOL_INTENT_RECORDED", map[string]interface{}{"step_id": step.StepID, "tool_name": intent.ToolName, "arguments_hash": intent.ArgumentsHash, "idempotency_key": intent.IdempotencyKey})
	if err != nil {
		return contracts.ToolResult{}, err
	}
	record, err := s.store.RecordToolIntentWithEvent(ctx, contracts.ToolCallRecord{ID: task.NewID("tcall"), Version: contracts.SchemaVersion, StepID: step.StepID, ToolName: intent.ToolName, ArgumentsHash: intent.ArgumentsHash, IdempotencyKey: intent.IdempotencyKey, Risk: intent.Risk, Status: "INTENT_RECORDED", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, event)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	registry := tool.NewRegistry()
	if err = tool.RegisterApprovedWriteFile(registry, workspaceItem.RootPath, func(map[string]interface{}) error {
		return s.store.ConsumeApproval(ctx, item.ID, step.StepID, intent.ToolName, intent.ArgumentsHash)
	}); err != nil {
		return contracts.ToolResult{}, err
	}
	result, invokeErr := tool.Invoke(registry, intent.ToolName)(ctx, writeArguments(input))
	if invokeErr != nil {
		return contracts.ToolResult{}, invokeErr
	}
	result.ToolCallID = record.ID
	if err = s.store.UpdateToolIntentStatus(ctx, record.ID, result.Status); err != nil {
		return contracts.ToolResult{}, err
	}
	return result, nil
}

func writeIntent(taskID string, input WriteFileInput) (policy.Intent, error) {
	return policy.NewIntent("write_file", writeArguments(input), contracts.RiskWrite, taskID+"\x00"+input.StepID)
}

func writeArguments(input WriteFileInput) map[string]interface{} {
	return map[string]interface{}{"path": input.Path, "content": input.Content, "expected_content_hash": input.ExpectedContentHash}
}

func validateWriteInput(input WriteFileInput) error {
	if strings.TrimSpace(input.TaskID) == "" || strings.TrimSpace(input.StepID) == "" || strings.TrimSpace(input.Path) == "" || strings.TrimSpace(input.ExpectedContentHash) == "" {
		return contracts.NewError(contracts.ErrInvalidInput, "task_id, step_id, path and expected_content_hash are required")
	}
	return nil
}

func (s *Service) writeContext(ctx context.Context, taskID, stepID string, requireRunning bool) (contracts.Task, contracts.StepSpec, contracts.Workspace, error) {
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, err
	}
	if item.Status.Terminal() {
		return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, contracts.NewError(contracts.ErrInvalidTransition, "cannot approve or execute a completed task")
	}
	activePlan, err := s.store.GetLatestPlan(ctx, taskID)
	if err != nil {
		return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, err
	}
	var step contracts.StepSpec
	found := false
	for _, candidate := range activePlan.Steps {
		if candidate.StepID == stepID {
			step, found = candidate, true
			break
		}
	}
	if !found {
		return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, contracts.NewError(contracts.ErrNotFound, "step is not in the active plan")
	}
	if !contains(step.AllowedTools, "write_file") || step.Risk != contracts.RiskWrite {
		return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, contracts.NewError(contracts.ErrToolNotAllowed, "write_file is not authorized for this step")
	}
	if requireRunning {
		if item.Status != contracts.TaskRunning {
			return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, contracts.NewError(contracts.ErrInvalidTransition, "write_file requires a running task")
		}
		states, stateErr := s.store.GetSteps(ctx, activePlan.PlanID)
		if stateErr != nil {
			return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, stateErr
		}
		for _, state := range states {
			if state.StepID == stepID && state.Status == contracts.StepRunning {
				workspaceItem, workspaceErr := s.store.GetWorkspace(ctx, item.WorkspaceID)
				return item, step, workspaceItem, workspaceErr
			}
		}
		return contracts.Task{}, contracts.StepSpec{}, contracts.Workspace{}, contracts.NewError(contracts.ErrInvalidTransition, "write_file requires a running step")
	}
	workspaceItem, err := s.store.GetWorkspace(ctx, item.WorkspaceID)
	return item, step, workspaceItem, err
}

func validateWriteScope(root string, scopes []string, path string) error {
	target, err := workspace.ResolveWithin(root, path)
	if err != nil {
		return err
	}
	for _, scope := range scopes {
		scopePath, scopeErr := workspace.ResolveWithin(root, scope)
		if scopeErr != nil {
			return scopeErr
		}
		relative, relErr := filepath.Rel(scopePath, target)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return nil
		}
	}
	return contracts.NewError(contracts.ErrPathDenied, "write path is outside the step workspace scopes")
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Service) CreateTask(ctx context.Context, input CreateTaskInput) (contracts.Task, contracts.PlanVersion, error) {
	if _, err := s.store.GetWorkspace(ctx, input.WorkspaceID); err != nil {
		return contracts.Task{}, contracts.PlanVersion{}, err
	}
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Goal) == "" {
		return contracts.Task{}, contracts.PlanVersion{}, contracts.NewError(contracts.ErrInvalidInput, "task title and goal are required")
	}
	if len(input.AcceptanceCriteria) == 0 {
		input.AcceptanceCriteria = []contracts.AcceptanceCriterion{{ID: "ac_evidence", Type: contracts.AcceptanceEvidenceExists, Description: "侦察报告已生成", Spec: map[string]interface{}{"kind": "RECON_REPORT"}}}
	}
	if input.Budget.MaxSteps == 0 {
		input.Budget.MaxSteps = 8
	}
	if input.Budget.MaxDurationMS == 0 {
		input.Budget.MaxDurationMS = int64((30 * time.Minute).Milliseconds())
	}
	if input.Budget.MaxReplans == 0 {
		input.Budget.MaxReplans = 2
	}
	now := time.Now().UTC()
	spec := contracts.TaskSpec{Version: contracts.SchemaVersion, TaskID: task.NewID("tsk"), WorkspaceID: input.WorkspaceID, Title: input.Title, Goal: input.Goal, Constraints: input.Constraints, AcceptanceCriteria: input.AcceptanceCriteria, Budget: input.Budget, CreatedAt: now}
	item := contracts.Task{ID: spec.TaskID, Version: contracts.SchemaVersion, WorkspaceID: spec.WorkspaceID, Title: spec.Title, Goal: spec.Goal, Status: contracts.TaskDraft, Spec: spec, CreatedAt: now, UpdatedAt: now}
	event, err := s.newEvent(ctx, item.ID, "TASK_CREATED", map[string]interface{}{"title": item.Title})
	if err != nil {
		return contracts.Task{}, contracts.PlanVersion{}, err
	}
	if err = s.store.CreateTask(ctx, item, event); err != nil {
		return contracts.Task{}, contracts.PlanVersion{}, err
	}
	if err = s.transitionTask(ctx, item.ID, contracts.TaskDraft, contracts.TaskPlanning, "TASK_STATUS_CHANGED"); err != nil {
		return contracts.Task{}, contracts.PlanVersion{}, err
	}
	planVersion := minimalPlan(item)
	validation := plan.Validate(planVersion, item.Spec.Budget.MaxSteps)
	if !validation.Valid() {
		return contracts.Task{}, contracts.PlanVersion{}, contracts.NewError(contracts.ErrPlanInvalid, strings.Join(validation.Errors, "; "))
	}
	event, err = s.newEvent(ctx, item.ID, "PLAN_CREATED", map[string]interface{}{"plan_id": planVersion.PlanID, "revision": planVersion.Revision})
	if err != nil {
		return contracts.Task{}, contracts.PlanVersion{}, err
	}
	if err = s.store.CreatePlan(ctx, planVersion, event); err != nil {
		return contracts.Task{}, contracts.PlanVersion{}, err
	}
	if err = s.transitionTask(ctx, item.ID, contracts.TaskPlanning, contracts.TaskReady, "TASK_STATUS_CHANGED"); err != nil {
		return contracts.Task{}, contracts.PlanVersion{}, err
	}
	item.Status = contracts.TaskReady
	item.UpdatedAt = time.Now().UTC()
	return item, planVersion, nil
}

func minimalPlan(item contracts.Task) contracts.PlanVersion {
	return contracts.PlanVersion{Version: contracts.SchemaVersion, PlanID: task.NewID("pln"), TaskID: item.ID, Revision: 1, Reason: "INITIAL_PLAN", Summary: "先在只读边界内侦察工作区并保存可验证报告", CreatedByAgent: "core-minimal-planner", CreatedAt: time.Now().UTC(), Steps: []contracts.StepSpec{{Version: contracts.SchemaVersion, StepID: task.NewID("stp"), Title: "侦察工作区", Goal: "收集授权工作区的文件清单，形成可复核的侦察报告", AllowedTools: []string{"list_files", "read_file", "search_text"}, WorkspaceScopes: []string{"."}, ExpectedOutputs: []contracts.ExpectedOutput{{Name: "recon_report", Type: "ARTIFACT", Required: true}}, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "ac_recon", Type: contracts.AcceptanceEvidenceExists, Description: "存在已验证的侦察报告", Spec: map[string]interface{}{"kind": "RECON_REPORT"}}}, Risk: contracts.RiskRead, Budget: contracts.StepBudget{MaxAttempts: 1, MaxIterations: 3, MaxDurationMS: int64((5 * time.Minute).Milliseconds()), MaxInputTokens: 8000, MaxOutputTokens: 2000}, ExecutionMode: "DETERMINISTIC", PreferredRole: "RECON"}}}
}

func (s *Service) RunTask(ctx context.Context, taskID string) (TaskSnapshot, error) {
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	if item.Status != contracts.TaskReady {
		return TaskSnapshot{}, contracts.NewError(contracts.ErrInvalidTransition, "only READY tasks can start in the foundation runtime")
	}
	activePlan, err := s.store.GetLatestPlan(ctx, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	states, err := s.store.GetSteps(ctx, activePlan.PlanID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	statusByID := map[string]contracts.StepStatus{}
	for _, state := range states {
		statusByID[state.StepID] = state.Status
	}
	ready := plan.ReadySteps(activePlan, statusByID)
	if len(ready) != 1 {
		return TaskSnapshot{}, contracts.NewError(contracts.ErrPlanInvalid, "foundation runner requires exactly one ready step")
	}
	if err = s.transitionTask(ctx, taskID, contracts.TaskReady, contracts.TaskRunning, "TASK_STATUS_CHANGED"); err != nil {
		return TaskSnapshot{}, err
	}
	stepID := ready[0]
	if err = s.transitionStep(ctx, taskID, stepID, contracts.StepPending, contracts.StepReady, "STEP_STATUS_CHANGED"); err != nil {
		return TaskSnapshot{}, err
	}
	if err = s.transitionStep(ctx, taskID, stepID, contracts.StepReady, contracts.StepRunning, "STEP_STATUS_CHANGED"); err != nil {
		return TaskSnapshot{}, err
	}
	if err = s.runReconnaissance(ctx, item, stepID); err != nil {
		_ = s.transitionTask(ctx, taskID, contracts.TaskRunning, contracts.TaskFailed, "TASK_STATUS_CHANGED")
		return TaskSnapshot{}, err
	}
	if err = s.transitionStep(ctx, taskID, stepID, contracts.StepRunning, contracts.StepVerifying, "STEP_STATUS_CHANGED"); err != nil {
		return TaskSnapshot{}, err
	}
	if err = s.transitionStep(ctx, taskID, stepID, contracts.StepVerifying, contracts.StepCompleted, "STEP_STATUS_CHANGED"); err != nil {
		return TaskSnapshot{}, err
	}
	if err = s.transitionTask(ctx, taskID, contracts.TaskRunning, contracts.TaskVerifying, "TASK_STATUS_CHANGED"); err != nil {
		return TaskSnapshot{}, err
	}
	report, err := s.VerifyTask(ctx, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	if err = s.persistFinalReport(ctx, report); err != nil {
		return TaskSnapshot{}, err
	}
	if !report.Passed {
		_ = s.transitionTask(ctx, taskID, contracts.TaskVerifying, contracts.TaskFailed, "TASK_STATUS_CHANGED")
		return s.GetTaskSnapshot(ctx, taskID)
	}
	if err = s.transitionTask(ctx, taskID, contracts.TaskVerifying, contracts.TaskCompleted, "TASK_STATUS_CHANGED"); err != nil {
		return TaskSnapshot{}, err
	}
	snapshot, err := s.GetTaskSnapshot(ctx, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	if len(snapshot.Events) > 0 {
		if err = s.store.SaveCheckpoint(ctx, task.NewID("chk"), taskID, snapshot.Events[len(snapshot.Events)-1].Sequence, snapshot); err != nil {
			return TaskSnapshot{}, err
		}
	}
	return snapshot, nil
}

func (s *Service) VerifyTask(ctx context.Context, taskID string) (verifier.FinalReport, error) {
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return verifier.FinalReport{}, err
	}
	activePlan, err := s.store.GetLatestPlan(ctx, taskID)
	if err != nil {
		return verifier.FinalReport{}, err
	}
	evidence, err := s.store.ListTaskEvidence(ctx, taskID)
	if err != nil {
		return verifier.FinalReport{}, err
	}
	workspaceItem, err := s.store.GetWorkspace(ctx, item.WorkspaceID)
	if err != nil {
		return verifier.FinalReport{}, err
	}
	return verifier.VerifyInWorkspace(item, activePlan, evidence, workspaceItem.RootPath), nil
}

func (s *Service) persistFinalReport(ctx context.Context, report verifier.FinalReport) error {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	item, err := s.artifactStore.Put("FINAL_REPORT", "application/json", "deterministic task verification report", report.TaskID, "", encoded)
	if err != nil {
		return err
	}
	return s.store.SaveArtifact(ctx, item)
}

func (s *Service) runReconnaissance(ctx context.Context, item contracts.Task, stepID string) error {
	workspaceItem, err := s.store.GetWorkspace(ctx, item.WorkspaceID)
	if err != nil {
		return err
	}
	registry := tool.NewRegistry()
	if err = tool.RegisterReadOnly(registry, workspaceItem.RootPath); err != nil {
		return err
	}
	definition, ok := registry.Definition("list_files")
	if !ok {
		return fmt.Errorf("list_files tool is not registered")
	}
	handler := registryHandler(registry, "list_files")
	result, err := handler(ctx, map[string]interface{}{"path": ".", "limit": 200})
	if err != nil {
		return err
	}
	if result.Status != "SUCCEEDED" {
		return contracts.NewError(contracts.ErrInvalidInput, result.Summary)
	}
	report := map[string]interface{}{"version": contracts.SchemaVersion, "workspace_id": workspaceItem.ID, "workspace_root": workspaceItem.RootPath, "task_id": item.ID, "step_id": stepID, "generated_at": time.Now().UTC(), "tool_definition": definition, "result": result}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	artifactItem, err := s.artifactStore.Put("RECON_REPORT", "application/json", "工作区侦察报告", item.ID, stepID, encoded)
	if err != nil {
		return err
	}
	if err = s.store.SaveArtifact(ctx, artifactItem); err != nil {
		return err
	}
	evidence := contracts.Evidence{ID: task.NewID("evd"), Kind: "RECON_REPORT", Claim: "已在授权工作区内生成侦察报告", ArtifactID: artifactItem.ID, Location: "$", VerificationMethod: "DETERMINISTIC_TOOL_RESULT", VerifiedAt: time.Now().UTC(), Confidence: 1}
	if err = s.store.SaveEvidence(ctx, evidence); err != nil {
		return err
	}
	return s.store.AttachStepResults(ctx, stepID, []string{artifactItem.ID}, []string{evidence.ID})
}

func registryHandler(registry *tool.Registry, name string) tool.Handler { // narrow access preserves Registry's lookup boundary.
	definition, _ := registry.Definition(name)
	_ = definition
	// The handler registration is deliberately private to package tool; use the
	// first-party executor facade until the worker loop owns invocation.
	return tool.Invoke(registry, name)
}

type TaskSnapshot struct {
	Task   contracts.Task            `json:"task"`
	Plan   contracts.PlanVersion     `json:"plan"`
	Steps  []contracts.StepRuntime   `json:"steps"`
	Events []contracts.EventEnvelope `json:"events"`
}

func (s *Service) GetTaskSnapshot(ctx context.Context, taskID string) (TaskSnapshot, error) {
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	activePlan, err := s.store.GetLatestPlan(ctx, taskID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	steps, err := s.store.GetSteps(ctx, activePlan.PlanID)
	if err != nil {
		return TaskSnapshot{}, err
	}
	events, err := s.store.Events(ctx, taskID, 0)
	if err != nil {
		return TaskSnapshot{}, err
	}
	return TaskSnapshot{Task: item, Plan: activePlan, Steps: steps, Events: events}, nil
}

func (s *Service) newEvent(ctx context.Context, taskID, eventType string, payload map[string]interface{}) (contracts.EventEnvelope, error) {
	return eventstore.NewTaskEvent(ctx, s.store, taskID, eventType, payload)
}
func (s *Service) transitionTask(ctx context.Context, taskID string, from, to contracts.TaskStatus, eventType string) error {
	if err := task.ValidateTransition(from, to); err != nil {
		return err
	}
	event, err := s.newEvent(ctx, taskID, eventType, map[string]interface{}{"from": from, "to": to})
	if err != nil {
		return err
	}
	return s.store.TransitionTask(ctx, taskID, from, to, event)
}
func (s *Service) transitionStep(ctx context.Context, taskID, stepID string, from, to contracts.StepStatus, eventType string) error {
	event, err := s.newEvent(ctx, taskID, eventType, map[string]interface{}{"step_id": stepID, "from": from, "to": to})
	if err != nil {
		return err
	}
	return s.store.TransitionStep(ctx, stepID, from, to, event)
}
