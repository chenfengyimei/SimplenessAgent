package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/contextpack"
	horizoncore "github.com/xm/simplenessagent/internal/horizon"
	"github.com/xm/simplenessagent/internal/tool"
	"github.com/xm/simplenessagent/pkg/contracts"
)

type CreateLongHorizonTaskInput struct {
	CreateTaskInput
	DeploymentID string
}

func (s *Service) CreateLongHorizonTask(ctx context.Context, input CreateLongHorizonTaskInput) (contracts.Task, contracts.HorizonState, error) {
	if input.CreateTaskInput.PermissionMode == contracts.PermissionModeDevelopment {
		return contracts.Task{}, contracts.HorizonState{}, contracts.NewError(contracts.ErrPermissionDenied, "long-horizon mode requires PLAN or approval-gated EDIT permission; DEVELOPMENT would bypass mandatory write confirmation")
	}
	deployment, err := s.store.GetDeployment(ctx, input.DeploymentID)
	if err != nil {
		return contracts.Task{}, contracts.HorizonState{}, err
	}
	if !deployment.Enabled {
		return contracts.Task{}, contracts.HorizonState{}, contracts.NewError(contracts.ErrPermissionDenied, "long-horizon deployment is disabled")
	}
	window, err := s.reliableContextWindow(ctx, deployment)
	if err != nil {
		return contracts.Task{}, contracts.HorizonState{}, err
	}
	if window < 8192 {
		return contracts.Task{}, contracts.HorizonState{}, contracts.NewError(contracts.ErrContextOverflow, "long-horizon mode requires a verified usable context window of at least 8192 tokens")
	}
	input.CreateTaskInput.DeploymentID = input.DeploymentID
	input.CreateTaskInput.ExecutionStrategy = contracts.ExecutionStrategyIncrementalHorizon
	if len(input.CreateTaskInput.AcceptanceCriteria) == 0 {
		input.CreateTaskInput.AcceptanceCriteria = []contracts.AcceptanceCriterion{{ID: "long-horizon-report", Type: contracts.AcceptanceEvidenceExists, Description: "at least one verified bounded Agent report exists", Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}
	}
	if input.CreateTaskInput.StageCheckpointPolicy == "" {
		input.CreateTaskInput.StageCheckpointPolicy = contracts.StageCheckpointKeyStages
	}
	created, _, err := s.CreateTask(ctx, input.CreateTaskInput)
	if err != nil {
		return contracts.Task{}, contracts.HorizonState{}, err
	}
	now := time.Now().UTC()
	horizonPlan := horizoncore.DefaultPlan(created.ID, now)
	if _, err = s.persistHorizonArtifact(ctx, created.ID, "", "HORIZON_PLAN", "deterministic five-stage software-engineering horizon", horizonPlan); err != nil {
		return contracts.Task{}, contracts.HorizonState{}, err
	}
	for _, profile := range horizoncore.DefaultProfiles(deployment.ID, now) {
		if err = s.store.UpsertModelRoleProfile(ctx, profile); err != nil {
			return contracts.Task{}, contracts.HorizonState{}, err
		}
	}
	state := contracts.HorizonState{Version: contracts.SchemaVersion, HorizonID: horizonPlan.HorizonID, TaskID: created.ID, Status: contracts.HorizonActive, Plan: horizonPlan, CurrentStageIndex: 0, StartedAt: now, DeadlineAt: now.Add(time.Duration(created.Spec.Budget.MaxDurationMS) * time.Millisecond), UpdatedAt: now}
	ledger := ledgerFor(created, state, nil)
	ledgerArtifact, err := s.persistHorizonArtifact(ctx, created.ID, "", "PROGRESS_LEDGER", "initial long-horizon progress ledger", ledger)
	if err != nil {
		return contracts.Task{}, contracts.HorizonState{}, err
	}
	state.LatestLedgerArtifactID = ledgerArtifact.ID
	if err = s.saveHorizonState(ctx, &state, "HORIZON_CREATED", map[string]interface{}{"stage": currentStage(state).ID, "context_window": window}); err != nil {
		return contracts.Task{}, contracts.HorizonState{}, err
	}
	return created, state, nil
}

func (s *Service) GetLongHorizonStatus(ctx context.Context, taskID string) (contracts.HorizonState, error) {
	state, err := s.store.GetHorizon(ctx, taskID)
	if err != nil {
		return state, err
	}
	if state.Version != contracts.SchemaVersion || state.HorizonID == "" || state.TaskID != taskID || len(state.Plan.Stages) != 5 {
		return contracts.HorizonState{}, contracts.NewError(contracts.ErrInvalidInput, "persisted long-horizon state is invalid")
	}
	return state, nil
}

// AdvanceLongHorizonTask performs one durable action: plan one segment,
// execute one step, or close one segment/stage. Callers may safely loop this
// method and can stop between any two calls without losing implicit state.
func (s *Service) AdvanceLongHorizonTask(ctx context.Context, taskID string) (contracts.LongHorizonCycleResult, error) {
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	if item.Spec.ExecutionStrategy != contracts.ExecutionStrategyIncrementalHorizon {
		return contracts.LongHorizonCycleResult{}, contracts.NewError(contracts.ErrInvalidInput, "task does not use incremental-horizon execution")
	}
	state, err := s.GetLongHorizonStatus(ctx, taskID)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	if item.Status.Terminal() || state.Status == contracts.HorizonCompleted || state.Status == contracts.HorizonCancelled || state.Status == contracts.HorizonBlocked {
		return cycleResult(item, state, "TERMINAL"), nil
	}
	if state.Status == contracts.HorizonWaiting && !state.AwaitingCheckpoint && (item.Status == contracts.TaskReady || item.Status == contracts.TaskRunning) {
		state.Status = contracts.HorizonActive
		state.CheckpointReason = ""
		if err = s.saveHorizonState(ctx, &state, "HORIZON_EXTERNAL_WAIT_RESOLVED", nil); err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
	}
	if item.Status == contracts.TaskWaitingApproval || item.Status == contracts.TaskWaitingUser || item.Status == contracts.TaskPaused {
		wanted := contracts.HorizonWaiting
		reason := "long-horizon task is waiting for user input"
		if item.Status == contracts.TaskWaitingApproval {
			reason = "a parameter-bound write or command is waiting for approval"
		}
		if item.Status == contracts.TaskPaused {
			wanted = contracts.HorizonPaused
			reason = "task execution is paused"
		}
		if state.Status != wanted || state.CheckpointReason == "" {
			state.Status = wanted
			if state.CheckpointReason == "" {
				state.CheckpointReason = reason
			}
			if err = s.saveHorizonState(ctx, &state, "HORIZON_WAITING", map[string]interface{}{"task_status": item.Status}); err != nil {
				return contracts.LongHorizonCycleResult{}, err
			}
		}
		return cycleResult(item, state, "WAITING"), nil
	}
	if state.Status == contracts.HorizonPaused || state.AwaitingCheckpoint {
		return cycleResult(item, state, "WAITING"), nil
	}
	if time.Now().UTC().After(state.DeadlineAt) || horizonTokenBudgetExhausted(item.Spec.Budget, state.Usage) {
		return s.pauseHorizonForBudget(ctx, item, state)
	}
	activePlan, err := s.store.GetLatestPlan(ctx, item.ID)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	if activePlan.HorizonID != state.HorizonID || activePlan.PlanID == state.LastProcessedPlanID {
		if state.StepsPlanned >= item.Spec.Budget.MaxSteps {
			return s.pauseHorizonForBudget(ctx, item, state)
		}
		return s.planNextHorizonSegment(ctx, item, state, activePlan)
	}
	steps, err := s.store.GetSteps(ctx, activePlan.PlanID)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	if hasRunnableStep(steps) {
		sections := s.longHorizonContextSections(ctx, item, state)
		snapshot, runErr := s.RunModelStep(ctx, RunModelStepInput{TaskID: item.ID, DeploymentID: item.Spec.DeploymentID, ContextSections: sections})
		if runErr != nil {
			return s.recordHorizonFailure(ctx, item, state, activePlan, runErr)
		}
		return s.recordHorizonProgress(ctx, snapshot.Task, state, activePlan, snapshot)
	}
	return s.finishHorizonSegment(ctx, item, state, activePlan, steps)
}

func horizonTokenBudgetExhausted(budget contracts.TaskBudget, usage contracts.TokenUsage) bool {
	return budget.MaxModelInputTokens > 0 && usage.InputTokens >= budget.MaxModelInputTokens ||
		budget.MaxModelOutputTokens > 0 && usage.OutputTokens >= budget.MaxModelOutputTokens
}

func (s *Service) ResumeLongHorizonTask(ctx context.Context, taskID string) (contracts.HorizonState, error) {
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return contracts.HorizonState{}, err
	}
	state, err := s.GetLongHorizonStatus(ctx, taskID)
	if err != nil {
		return contracts.HorizonState{}, err
	}
	switch item.Status {
	case contracts.TaskWaitingUser:
		if !state.AwaitingCheckpoint {
			planVersion, planErr := s.store.GetLatestPlan(ctx, item.ID)
			if planErr != nil {
				return state, planErr
			}
			steps, stepErr := s.store.GetSteps(ctx, planVersion.PlanID)
			if stepErr != nil {
				return state, stepErr
			}
			for _, stepState := range steps {
				if stepState.Status == contracts.StepWaitingUser {
					if stepErr = s.transitionStep(ctx, item.ID, stepState.StepID, contracts.StepWaitingUser, contracts.StepFailed, "HORIZON_USER_ANSWER_REPLAN"); stepErr != nil {
						return state, stepErr
					}
				}
			}
		}
		err = s.transitionTask(ctx, item.ID, contracts.TaskWaitingUser, contracts.TaskRunning, "HORIZON_CHECKPOINT_APPROVED")
	case contracts.TaskPaused:
		err = s.transitionTask(ctx, item.ID, contracts.TaskPaused, contracts.TaskReady, "HORIZON_RESUMED")
	case contracts.TaskReady, contracts.TaskRunning:
		// Idempotent resume.
	case contracts.TaskWaitingApproval:
		return state, contracts.NewError(contracts.ErrApprovalRequired, "pending write or command approval must be resolved before resume")
	default:
		return state, contracts.NewError(contracts.ErrInvalidTransition, "long-horizon task cannot resume from its current state")
	}
	if err != nil {
		return state, err
	}
	state.Status = contracts.HorizonActive
	state.AwaitingCheckpoint = false
	state.CheckpointReason = ""
	if err = s.saveHorizonState(ctx, &state, "HORIZON_RESUMED", nil); err != nil {
		return state, err
	}
	return state, nil
}

func (s *Service) CancelLongHorizonTask(ctx context.Context, taskID string) (contracts.HorizonState, error) {
	item, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return contracts.HorizonState{}, err
	}
	state, err := s.GetLongHorizonStatus(ctx, taskID)
	if err != nil {
		return contracts.HorizonState{}, err
	}
	if !item.Status.Terminal() {
		if err = s.transitionTask(ctx, item.ID, item.Status, contracts.TaskCancelled, "HORIZON_CANCELLED"); err != nil {
			return state, err
		}
	}
	state.Status = contracts.HorizonCancelled
	if err = s.saveHorizonState(ctx, &state, "HORIZON_STATE_CANCELLED", nil); err != nil {
		return state, err
	}
	return state, nil
}

func (s *Service) planNextHorizonSegment(ctx context.Context, item contracts.Task, state contracts.HorizonState, previous contracts.PlanVersion) (contracts.LongHorizonCycleResult, error) {
	deployment, err := s.store.GetDeployment(ctx, item.Spec.DeploymentID)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	provider, err := s.resolveProvider(deployment)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	workspaceItem, err := s.store.GetWorkspace(ctx, item.WorkspaceID)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	registry := tool.NewRegistry()
	if err = s.registerPlannerTools(registry, workspaceItem.RootPath, item); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	definitions := stageToolDefinitions(currentStage(state).ID, registry.Definitions())
	profile, err := s.store.GetModelRoleProfile(ctx, deployment.ID, contracts.ModelRolePlanner)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	planner, err := horizoncore.NewSegmentPlanner(provider)
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	ledger := s.readProgressLedger(ctx, item, state)
	segment, usage, err := planner.Create(ctx, horizoncore.SegmentInput{DeploymentID: deployment.ID, Task: item, Horizon: state, Stage: currentStage(state), Ledger: ledger, Tools: definitions, Profile: profile, Revision: previous.Revision + 1, ParentPlanID: previous.PlanID})
	state.Usage.InputTokens += usage.InputTokens
	state.Usage.OutputTokens += usage.OutputTokens
	if err != nil {
		return s.recordHorizonFailure(ctx, item, state, previous, err)
	}
	if err = validatePlanPermission(segment, item); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	if err = s.validateHorizonSegmentNovelty(ctx, item.ID, segment); err != nil {
		return s.recordHorizonFailure(ctx, item, state, previous, err)
	}
	event, err := s.newEvent(ctx, item.ID, "HORIZON_SEGMENT_PLANNED", map[string]interface{}{"plan_id": segment.PlanID, "stage": segment.StageID, "segment": segment.SegmentIndex, "steps": len(segment.Steps)})
	if err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	if err = s.store.CreatePlan(ctx, segment, event); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	state.SegmentIndex = segment.SegmentIndex
	state.StepsPlanned += len(segment.Steps)
	state.NoProgressCycles++
	if err = s.persistLedgerAndState(ctx, item, &state, "HORIZON_SEGMENT_READY"); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	return cycleResult(item, state, "SEGMENT_PLANNED"), nil
}

func (s *Service) validateHorizonSegmentNovelty(ctx context.Context, taskID string, segment contracts.PlanVersion) error {
	plans, err := s.store.ListTaskPlans(ctx, taskID)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, prior := range plans {
		if prior.HorizonID == "" || prior.StageID != segment.StageID {
			continue
		}
		for _, step := range prior.Steps {
			seen[horizonStepIntentKey(step)] = true
		}
	}
	for _, step := range segment.Steps {
		key := horizonStepIntentKey(step)
		if seen[key] {
			return contracts.NewError(contracts.ErrRepeatedAction, "incremental plan repeats a previously planned step in the same stage")
		}
		seen[key] = true
	}
	return nil
}

func horizonStepIntentKey(step contracts.StepSpec) string {
	tools := append([]string{}, step.AllowedTools...)
	sort.Strings(tools)
	return strings.ToLower(strings.Join(strings.Fields(step.Goal), " ")) + "\x00" + strings.Join(tools, "\x00")
}

func (s *Service) recordHorizonProgress(ctx context.Context, item contracts.Task, state contracts.HorizonState, activePlan contracts.PlanVersion, snapshot TaskSnapshot) (contracts.LongHorizonCycleResult, error) {
	state.StepsCompleted = countCompletedHorizonSteps(snapshot.Events)
	evidence, _ := s.store.ListTaskEvidence(ctx, item.ID)
	if len(evidence) > state.LastEvidenceCount {
		state.NoProgressCycles = 0
	} else {
		state.NoProgressCycles++
	}
	state.LastEvidenceCount = len(evidence)
	if report, ok := s.latestAgentReport(ctx, item.ID); ok {
		state.Usage.InputTokens += report.Usage.InputTokens
		state.Usage.OutputTokens += report.Usage.OutputTokens
		if report.ContextRecompilations > 0 {
			if err := s.saveHorizonState(ctx, &state, "HORIZON_CONTEXT_RECOMPILED", map[string]interface{}{"step_id": report.StepID, "count": report.ContextRecompilations}); err != nil {
				return contracts.LongHorizonCycleResult{}, err
			}
		}
	}
	if state.NoProgressCycles >= 3 {
		state.Status = contracts.HorizonPaused
		state.CheckpointReason = "three consecutive cycles produced no new evidence"
		if item.Status == contracts.TaskRunning {
			_ = s.transitionTask(ctx, item.ID, contracts.TaskRunning, contracts.TaskPaused, "HORIZON_NO_PROGRESS_PAUSED")
		}
	}
	if item.Status == contracts.TaskCompleted {
		state.Status = contracts.HorizonCompleted
	} else if item.Status == contracts.TaskFailed || item.Status == contracts.TaskBlocked {
		state.Status = contracts.HorizonBlocked
		state.CheckpointReason = "global deterministic acceptance did not pass"
	}
	if err := s.persistLedgerAndState(ctx, item, &state, "HORIZON_PROGRESS_RECORDED"); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	return cycleResult(item, state, "STEP_EXECUTED"), nil
}

func (s *Service) finishHorizonSegment(ctx context.Context, item contracts.Task, state contracts.HorizonState, activePlan contracts.PlanVersion, steps []contracts.StepRuntime) (contracts.LongHorizonCycleResult, error) {
	allCompleted := true
	for _, stepState := range steps {
		if stepState.Status != contracts.StepCompleted {
			allCompleted = false
			break
		}
	}
	state.LastProcessedPlanID = activePlan.PlanID
	if !allCompleted {
		state.ReplansUsed++
		if state.ReplansUsed > item.Spec.Budget.MaxReplans {
			state.Status = contracts.HorizonBlocked
			state.CheckpointReason = "replan budget exhausted"
			if item.Status == contracts.TaskRunning {
				_ = s.transitionTask(ctx, item.ID, contracts.TaskRunning, contracts.TaskBlocked, "HORIZON_REPLAN_EXHAUSTED")
			}
		} else {
			state.Status = contracts.HorizonActive
		}
		if err := s.persistLedgerAndState(ctx, item, &state, "HORIZON_REPLAN_REQUIRED"); err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
		return cycleResult(item, state, "REPLAN_REQUIRED"), nil
	}
	if !activePlan.TerminalSegment {
		if err := s.persistLedgerAndState(ctx, item, &state, "HORIZON_SEGMENT_COMPLETED"); err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
		return cycleResult(item, state, "SEGMENT_COMPLETED"), nil
	}
	stage := currentStage(state)
	opinion, usage, err := s.assessHorizonStage(ctx, item, stage, steps)
	if err != nil {
		return s.recordHorizonFailure(ctx, item, state, activePlan, err)
	}
	state.Usage.InputTokens += usage.InputTokens
	state.Usage.OutputTokens += usage.OutputTokens
	if _, err := s.persistHorizonArtifact(ctx, item.ID, "", "STAGE_SUMMARY", "completed long-horizon stage "+string(stage.ID), map[string]interface{}{"stage": stage, "plan_id": activePlan.PlanID, "steps": steps, "verifier_opinion": opinion, "deterministic_gate": "all persisted stage steps completed", "completed_at": time.Now().UTC()}); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	state.CurrentStageIndex++
	if state.CurrentStageIndex >= len(state.Plan.Stages) {
		if item.Status != contracts.TaskRunning {
			return contracts.LongHorizonCycleResult{}, contracts.NewError(contracts.ErrInvalidTransition, "FINALIZE can only close a running task")
		}
		if err := s.transitionTask(ctx, item.ID, contracts.TaskRunning, contracts.TaskVerifying, "TASK_STATUS_CHANGED"); err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
		report, err := s.VerifyTask(ctx, item.ID)
		if err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
		if err = s.persistFinalReport(ctx, report); err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
		if report.Passed {
			if err = s.transitionTask(ctx, item.ID, contracts.TaskVerifying, contracts.TaskCompleted, "TASK_STATUS_CHANGED"); err != nil {
				return contracts.LongHorizonCycleResult{}, err
			}
			item.Status = contracts.TaskCompleted
			state.Status = contracts.HorizonCompleted
			if err = s.persistLedgerAndState(ctx, item, &state, "HORIZON_COMPLETED"); err != nil {
				return contracts.LongHorizonCycleResult{}, err
			}
			return cycleResult(item, state, "COMPLETED"), nil
		}
		if err = s.transitionTask(ctx, item.ID, contracts.TaskVerifying, contracts.TaskFailed, "TASK_STATUS_CHANGED"); err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
		item.Status = contracts.TaskFailed
		state.Status = contracts.HorizonBlocked
		state.CheckpointReason = "FINALIZE deterministic acceptance failed"
		if err = s.persistLedgerAndState(ctx, item, &state, "HORIZON_FINAL_ACCEPTANCE_FAILED"); err != nil {
			return contracts.LongHorizonCycleResult{}, err
		}
		return cycleResult(item, state, "FINAL_ACCEPTANCE_FAILED"), nil
	}
	if requiresStageCheckpoint(item.Spec.StageCheckpointPolicy, stage.ID) {
		state.Status = contracts.HorizonWaiting
		state.AwaitingCheckpoint = true
		state.CheckpointReason = "stage " + string(stage.ID) + " completed; approve before " + string(currentStage(state).ID)
		if item.Status == contracts.TaskRunning {
			if err := s.transitionTask(ctx, item.ID, contracts.TaskRunning, contracts.TaskWaitingUser, "HORIZON_STAGE_CHECKPOINT"); err != nil {
				return contracts.LongHorizonCycleResult{}, err
			}
		}
	}
	if err := s.persistLedgerAndState(ctx, item, &state, "HORIZON_STAGE_ADVANCED"); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	return cycleResult(item, state, "STAGE_ADVANCED"), nil
}

func (s *Service) assessHorizonStage(ctx context.Context, item contracts.Task, stage contracts.HorizonStage, steps []contracts.StepRuntime) (contracts.StageVerificationOpinion, contracts.TokenUsage, error) {
	deployment, err := s.store.GetDeployment(ctx, item.Spec.DeploymentID)
	if err != nil {
		return contracts.StageVerificationOpinion{}, contracts.TokenUsage{}, err
	}
	provider, err := s.resolveProvider(deployment)
	if err != nil {
		return contracts.StageVerificationOpinion{}, contracts.TokenUsage{}, err
	}
	profile, err := s.store.GetModelRoleProfile(ctx, deployment.ID, contracts.ModelRoleVerifier)
	if err != nil {
		return contracts.StageVerificationOpinion{}, contracts.TokenUsage{}, err
	}
	verifier, err := horizoncore.NewStageVerifier(provider)
	if err != nil {
		return contracts.StageVerificationOpinion{}, contracts.TokenUsage{}, err
	}
	evidence, err := s.store.ListTaskEvidence(ctx, item.ID)
	if err != nil {
		return contracts.StageVerificationOpinion{}, contracts.TokenUsage{}, err
	}
	return verifier.Assess(ctx, horizoncore.StageVerificationInput{DeploymentID: deployment.ID, Goal: item.Goal, Stage: stage, Steps: steps, Evidence: evidence, Profile: profile})
}

func (s *Service) recordHorizonFailure(ctx context.Context, item contracts.Task, state contracts.HorizonState, activePlan contracts.PlanVersion, cause error) (contracts.LongHorizonCycleResult, error) {
	code := string(contracts.ErrProviderInternal)
	if domain, ok := cause.(*contracts.Error); ok {
		code = string(domain.Code)
	}
	artifactItem, persistErr := s.persistHorizonArtifact(ctx, item.ID, "", "FAILURE_REPORT", "long-horizon cycle failure", map[string]interface{}{"stage": currentStage(state).ID, "plan_id": activePlan.PlanID, "error_code": code, "message": cause.Error(), "attempted_plan_id": activePlan.PlanID, "prohibited_replay": true, "next_action": "resume only after reviewing this failure; repeated unsupported work is blocked", "recorded_at": time.Now().UTC()})
	if persistErr != nil {
		return contracts.LongHorizonCycleResult{}, persistErr
	}
	state.LatestFailureArtifactID = artifactItem.ID
	state.Status = contracts.HorizonPaused
	state.CheckpointReason = boundedCheckpointReason("cycle failed: "+cause.Error(), 600)
	state.ReplansUsed++
	latestTask, taskErr := s.store.GetTask(ctx, item.ID)
	if taskErr != nil {
		return contracts.LongHorizonCycleResult{}, taskErr
	}
	if latestTask.Status == contracts.TaskReady || latestTask.Status == contracts.TaskRunning {
		if taskErr = s.transitionTask(ctx, item.ID, latestTask.Status, contracts.TaskPaused, "HORIZON_FAILURE_PAUSED"); taskErr != nil {
			return contracts.LongHorizonCycleResult{}, taskErr
		}
		item.Status = contracts.TaskPaused
	}
	if err := s.persistLedgerAndState(ctx, item, &state, "HORIZON_FAILURE_RECORDED"); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	// A model/schema/tool failure is a durable long-horizon outcome, not a lost
	// service operation. The failure artifact and PAUSED checkpoint let desktop
	// and CLI callers show the exact reason and resume explicitly.
	return cycleResult(item, state, "FAILED_PAUSED"), nil
}

func boundedCheckpointReason(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func (s *Service) pauseHorizonForBudget(ctx context.Context, item contracts.Task, state contracts.HorizonState) (contracts.LongHorizonCycleResult, error) {
	state.Status = contracts.HorizonBlocked
	state.CheckpointReason = "long-horizon time or step budget exhausted"
	if item.Status == contracts.TaskRunning {
		_ = s.transitionTask(ctx, item.ID, contracts.TaskRunning, contracts.TaskBlocked, "HORIZON_BUDGET_EXHAUSTED")
	} else if item.Status == contracts.TaskReady {
		_ = s.transitionTask(ctx, item.ID, contracts.TaskReady, contracts.TaskPaused, "HORIZON_BUDGET_EXHAUSTED")
	}
	if err := s.saveHorizonState(ctx, &state, "HORIZON_BUDGET_BLOCKED", nil); err != nil {
		return contracts.LongHorizonCycleResult{}, err
	}
	return cycleResult(item, state, "BUDGET_BLOCKED"), nil
}

func (s *Service) longHorizonContextSections(ctx context.Context, item contracts.Task, state contracts.HorizonState) []contracts.ContextSection {
	sections := []contracts.ContextSection{}
	if content, ok := s.readArtifactByID(ctx, item.ID, state.LatestLedgerArtifactID); ok {
		sections = append(sections, contracts.ContextSection{Type: "PROGRESS_LEDGER", Content: string(content), SourceRefs: []string{state.LatestLedgerArtifactID}, Priority: 99})
	}
	if content, ok := s.readArtifactByID(ctx, item.ID, state.LatestFailureArtifactID); ok {
		sections = append(sections, contracts.ContextSection{Type: "LATEST_FAILURE", Content: string(content), SourceRefs: []string{state.LatestFailureArtifactID}, Priority: 98})
	}
	if item.ConversationID != "" {
		if messages, err := s.store.ListConversationMessages(ctx, item.ConversationID); err == nil {
			history := make([]contracts.ContextSection, 0, len(messages))
			for _, message := range messages {
				history = append(history, contracts.ContextSection{Type: "CHAT_" + strings.ToUpper(message.Role), Content: message.Content, SourceRefs: []string{message.ID}, Priority: 80})
			}
			compressed, _ := contextpack.CompressHistory(history, 4, "CONVERSATION_SUMMARY")
			sections = append(sections, compressed...)
		}
	}
	if memories, err := s.store.SearchMemory(ctx, item.WorkspaceID, item.Goal, 4); err == nil {
		for _, memory := range memories {
			sections = append(sections, contracts.ContextSection{Type: "PROJECT_MEMORY", Content: memory.Title + "\n" + memory.Content, SourceRefs: []string{"memory:" + memory.ID}, Priority: 75})
		}
	}
	return sections
}

func (s *Service) persistLedgerAndState(ctx context.Context, item contracts.Task, state *contracts.HorizonState, eventType string) error {
	var snapshot *TaskSnapshot
	if current, err := s.GetTaskSnapshot(ctx, item.ID); err == nil {
		snapshot = &current
	}
	ledger := ledgerFor(item, *state, snapshot)
	artifactItem, err := s.persistHorizonArtifact(ctx, item.ID, "", "PROGRESS_LEDGER", "updated long-horizon progress ledger", ledger)
	if err != nil {
		return err
	}
	state.LatestLedgerArtifactID = artifactItem.ID
	return s.saveHorizonState(ctx, state, eventType, map[string]interface{}{"stage": ledger.CurrentStage, "steps_planned": state.StepsPlanned, "steps_completed": state.StepsCompleted})
}

func (s *Service) saveHorizonState(ctx context.Context, state *contracts.HorizonState, eventType string, payload map[string]interface{}) error {
	state.UpdatedAt = time.Now().UTC()
	event, err := s.newEvent(ctx, state.TaskID, eventType, payload)
	if err != nil {
		return err
	}
	return s.store.SaveHorizon(ctx, *state, event)
}

func (s *Service) persistHorizonArtifact(ctx context.Context, taskID, stepID, kind, summary string, value interface{}) (contracts.Artifact, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return contracts.Artifact{}, err
	}
	item, err := s.artifactStore.Put(kind, "application/json", summary, taskID, stepID, encoded)
	if err != nil {
		return contracts.Artifact{}, err
	}
	if err = s.store.SaveArtifact(ctx, item); err != nil {
		return contracts.Artifact{}, err
	}
	return item, nil
}

func (s *Service) readArtifactByID(ctx context.Context, taskID, artifactID string) ([]byte, bool) {
	if artifactID == "" {
		return nil, false
	}
	items, err := s.store.ListTaskArtifacts(ctx, taskID)
	if err != nil {
		return nil, false
	}
	for _, item := range items {
		if item.ID == artifactID {
			content, readErr := s.artifactStore.Read(item)
			return content, readErr == nil
		}
	}
	return nil, false
}

func (s *Service) readProgressLedger(ctx context.Context, item contracts.Task, state contracts.HorizonState) contracts.ProgressLedger {
	if content, ok := s.readArtifactByID(ctx, item.ID, state.LatestLedgerArtifactID); ok {
		var ledger contracts.ProgressLedger
		if json.Unmarshal(content, &ledger) == nil {
			return ledger
		}
	}
	return ledgerFor(item, state, nil)
}

func (s *Service) latestAgentReport(ctx context.Context, taskID string) (contracts.AgentReport, bool) {
	items, err := s.store.ListTaskArtifacts(ctx, taskID)
	if err != nil {
		return contracts.AgentReport{}, false
	}
	for index := len(items) - 1; index >= 0; index-- {
		if items[index].Kind != "AGENT_REPORT" {
			continue
		}
		content, readErr := s.artifactStore.Read(items[index])
		var report contracts.AgentReport
		if readErr == nil && json.Unmarshal(content, &report) == nil {
			return report, true
		}
	}
	return contracts.AgentReport{}, false
}

func ledgerFor(item contracts.Task, state contracts.HorizonState, snapshot *TaskSnapshot) contracts.ProgressLedger {
	completed, failed := []string{}, []string{}
	if snapshot != nil {
		completedSeen, failedSeen := map[string]bool{}, map[string]bool{}
		for _, event := range snapshot.Events {
			if event.EventType != "STEP_STATUS_CHANGED" && event.EventType != "HORIZON_USER_ANSWER_REPLAN" {
				continue
			}
			stepID := fmt.Sprint(event.Payload["step_id"])
			to := fmt.Sprint(event.Payload["to"])
			if stepID == "" {
				continue
			}
			if to == string(contracts.StepCompleted) && !completedSeen[stepID] {
				completedSeen[stepID] = true
				completed = append(completed, stepID)
			}
			if (to == string(contracts.StepFailed) || to == string(contracts.StepBlocked)) && !failedSeen[stepID] {
				failedSeen[stepID] = true
				failed = append(failed, stepID)
			}
		}
	}
	completedStages := make([]contracts.HorizonStageID, 0, state.CurrentStageIndex)
	for index := 0; index < state.CurrentStageIndex && index < len(state.Plan.Stages); index++ {
		completedStages = append(completedStages, state.Plan.Stages[index].ID)
	}
	stage := contracts.HorizonStageFinalize
	if state.CurrentStageIndex < len(state.Plan.Stages) {
		stage = state.Plan.Stages[state.CurrentStageIndex].ID
	}
	return contracts.ProgressLedger{Version: contracts.SchemaVersion, HorizonID: state.HorizonID, TaskID: item.ID, CurrentStage: stage, CompletedStages: completedStages, CompletedStepIDs: completed, FailedStepIDs: failed, LatestFailureCode: state.CheckpointReason, RemainingSteps: maxInt(0, item.Spec.Budget.MaxSteps-state.StepsPlanned), RemainingReplans: maxInt(0, item.Spec.Budget.MaxReplans-state.ReplansUsed), InputTokensUsed: state.Usage.InputTokens, OutputTokensUsed: state.Usage.OutputTokens, UpdatedAt: time.Now().UTC()}
}

func currentStage(state contracts.HorizonState) contracts.HorizonStage {
	if state.CurrentStageIndex >= 0 && state.CurrentStageIndex < len(state.Plan.Stages) {
		return state.Plan.Stages[state.CurrentStageIndex]
	}
	return contracts.HorizonStage{ID: contracts.HorizonStageFinalize, Title: "Finalize", Goal: "Finalize task"}
}

func stageToolDefinitions(stage contracts.HorizonStageID, definitions []contracts.ToolDefinition) []contracts.ToolDefinition {
	result := []contracts.ToolDefinition{}
	for _, definition := range definitions {
		allowed := definition.RiskClass == contracts.RiskRead
		if stage == contracts.HorizonStageImplement {
			allowed = true
		}
		if (stage == contracts.HorizonStageVerifyRepair || stage == contracts.HorizonStageFinalize) && strings.Contains(definition.Name, "project_command") {
			allowed = true
		}
		if allowed {
			result = append(result, definition)
		}
	}
	return result
}

func requiresStageCheckpoint(policy contracts.StageCheckpointPolicy, completed contracts.HorizonStageID) bool {
	return policy == contracts.StageCheckpointEveryStage || policy == contracts.StageCheckpointKeyStages && completed == contracts.HorizonStageDesign
}

func hasRunnableStep(steps []contracts.StepRuntime) bool {
	for _, stepState := range steps {
		if stepState.Status == contracts.StepPending || stepState.Status == contracts.StepReady || stepState.Status == contracts.StepRunning {
			return true
		}
	}
	return false
}

func countCompletedHorizonSteps(events []contracts.EventEnvelope) int {
	seen := map[string]bool{}
	for _, event := range events {
		if event.EventType == "STEP_STATUS_CHANGED" && fmt.Sprint(event.Payload["to"]) == string(contracts.StepCompleted) {
			seen[fmt.Sprint(event.Payload["step_id"])] = true
		}
	}
	return len(seen)
}

func cycleResult(item contracts.Task, state contracts.HorizonState, action string) contracts.LongHorizonCycleResult {
	return contracts.LongHorizonCycleResult{Version: contracts.SchemaVersion, TaskID: item.ID, HorizonID: state.HorizonID, Status: state.Status, Stage: currentStage(state).ID, Action: action, StepsPlanned: state.StepsPlanned, StepsCompleted: state.StepsCompleted, RemainingSteps: maxInt(0, item.Spec.Budget.MaxSteps-state.StepsPlanned), AwaitingCheckpoint: state.AwaitingCheckpoint, CheckpointReason: state.CheckpointReason}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
