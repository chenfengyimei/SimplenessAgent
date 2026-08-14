package horizon

import (
	"context"
	"testing"
	"time"

	"github.com/xm/simplenessagent/internal/provider/mock"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestSegmentPlannerBuildsBoundedLocalPlan(t *testing.T) {
	provider := mock.Provider{Response: `{"summary":"inspect first","terminal_segment":true,"steps":[{"ref":"scan","title":"Scan files","goal":"Locate the implementation","tool_intents":["list_files"],"acceptance_intent":"Relevant files are identified"},{"ref":"read","title":"Read target","goal":"Inspect the target implementation","dependencies":["scan"],"tool_intents":["read_file"],"acceptance_intent":"Current behavior is evidenced"}]}`}
	planner, err := NewSegmentPlanner(provider)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	plan := DefaultPlan("task-1", now)
	taskItem := contracts.Task{ID: "task-1", Goal: "fix the bug", Spec: contracts.TaskSpec{TaskID: "task-1", Budget: contracts.TaskBudget{MaxSteps: 20, MaxSegmentSteps: 4}}}
	state := contracts.HorizonState{HorizonID: plan.HorizonID, TaskID: taskItem.ID, Plan: plan}
	tools := []contracts.ToolDefinition{{Name: "list_files", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}, {Name: "read_file", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}}
	profile := DefaultProfiles("dep", now)[0]
	result, _, err := planner.Create(context.Background(), SegmentInput{DeploymentID: "dep", Task: taskItem, Horizon: state, Stage: plan.Stages[0], Ledger: contracts.ProgressLedger{}, Tools: tools, Profile: profile, Revision: 2, ParentPlanID: "initial"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Steps) != 2 || result.HorizonID != plan.HorizonID || result.StageID != "DISCOVER" || !result.TerminalSegment {
		t.Fatalf("unexpected segment plan: %#v", result)
	}
	if len(result.Steps[1].Dependencies) != 1 || result.Steps[1].Dependencies[0] != result.Steps[0].StepID || result.Steps[0].Budget.MaxOutputTokens != 768 {
		t.Fatalf("local identities or budgets were not applied: %#v", result.Steps)
	}
}

func TestSegmentPlannerRejectsOverBroadToolSetAfterRepair(t *testing.T) {
	provider := mock.Provider{Response: `{"summary":"bad","terminal_segment":true,"steps":[{"ref":"x","title":"x","goal":"x","tool_intents":["one","two","three","four"],"acceptance_intent":"x"}]}`}
	planner, _ := NewSegmentPlanner(provider)
	now := time.Now().UTC()
	plan := DefaultPlan("task", now)
	definitions := []contracts.ToolDefinition{}
	for _, name := range []string{"one", "two", "three", "four"} {
		definitions = append(definitions, contracts.ToolDefinition{Name: name, RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}})
	}
	_, _, err := planner.Create(context.Background(), SegmentInput{DeploymentID: "dep", Task: contracts.Task{ID: "task", Goal: "goal", Spec: contracts.TaskSpec{TaskID: "task", Budget: contracts.TaskBudget{MaxSteps: 20, MaxSegmentSteps: 4}}}, Horizon: contracts.HorizonState{HorizonID: plan.HorizonID, Plan: plan}, Stage: plan.Stages[0], Tools: definitions, Profile: DefaultProfiles("dep", now)[0], Revision: 2})
	if domain, ok := err.(*contracts.Error); !ok || domain.Code != contracts.ErrPlanInvalid {
		t.Fatalf("expected bounded plan rejection, got %v", err)
	}
}

func TestSegmentPlannerRejectsRepeatedStepIntent(t *testing.T) {
	provider := mock.Provider{Response: `{"summary":"repeat","terminal_segment":true,"steps":[{"ref":"a","title":"first","goal":"Read the same file","tool_intents":["read_file"],"acceptance_intent":"evidence"},{"ref":"b","title":"second","goal":"Read  the SAME file","tool_intents":["read_file"],"acceptance_intent":"evidence"}]}`}
	planner, _ := NewSegmentPlanner(provider)
	now := time.Now().UTC()
	plan := DefaultPlan("task", now)
	_, _, err := planner.Create(context.Background(), SegmentInput{DeploymentID: "dep", Task: contracts.Task{ID: "task", Goal: "goal", Spec: contracts.TaskSpec{TaskID: "task", Budget: contracts.TaskBudget{MaxSteps: 20, MaxSegmentSteps: 4}}}, Horizon: contracts.HorizonState{HorizonID: plan.HorizonID, Plan: plan}, Stage: plan.Stages[0], Tools: []contracts.ToolDefinition{{Name: "read_file", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}}, Profile: DefaultProfiles("dep", now)[0], Revision: 2})
	if domain, ok := err.(*contracts.Error); !ok || domain.Code != contracts.ErrPlanInvalid {
		t.Fatalf("expected repeated step intent to fail after repair, got %v", err)
	}
}
