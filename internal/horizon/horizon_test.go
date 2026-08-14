package horizon

import (
	"context"
	"strings"
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

func TestDecodeCandidateAcceptsThinkingPrefixWrapperAndCommonAliases(t *testing.T) {
	response := `<think>{"analysis":"inspect before planning"}</think>
~~~json
{"candidate":{"summary":"inspect safely","terminal":true,"metadata":{"model":"small"},"steps":[{"id":"scan","title":"Scan files","objective":"Locate the implementation","dependsOn":[],"tool_intent":"list_files","acceptance":"Relevant files are identified","confidence":0.8}]}}
~~~`
	candidate, err := decodeCandidate(response)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Summary != "inspect safely" || !candidate.TerminalSegment || len(candidate.Steps) != 1 {
		t.Fatalf("unexpected normalized candidate: %#v", candidate)
	}
	step := candidate.Steps[0]
	if step.Ref != "scan" || step.Goal != "Locate the implementation" || len(step.ToolIntents) != 1 || step.ToolIntents[0] != "list_files" || step.AcceptanceIntent != "Relevant files are identified" {
		t.Fatalf("common aliases were not normalized: %#v", step)
	}
}

func TestSegmentPlannerRepairIncludesSpecificSchemaError(t *testing.T) {
	provider := &plannerScriptProvider{responses: []string{
		`{"summary":7,"terminal_segment":true,"steps":[]}`,
		`{"summary":"repaired","terminal_segment":true,"steps":[{"ref":"scan","title":"Scan","goal":"Locate files","dependencies":[],"tool_intents":["list_files"],"acceptance_intent":"Files are identified"}]}`,
	}}
	planner, err := NewSegmentPlanner(provider)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	horizonPlan := DefaultPlan("task", now)
	result, _, err := planner.Create(context.Background(), SegmentInput{
		DeploymentID: "dep",
		Task:         contracts.Task{ID: "task", Goal: "goal", Spec: contracts.TaskSpec{TaskID: "task", Budget: contracts.TaskBudget{MaxSteps: 20, MaxSegmentSteps: 4}}},
		Horizon:      contracts.HorizonState{HorizonID: horizonPlan.HorizonID, Plan: horizonPlan},
		Stage:        horizonPlan.Stages[0],
		Tools:        []contracts.ToolDefinition{{Name: "list_files", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}},
		Profile:      DefaultProfiles("dep", now)[0],
		Revision:     2,
	})
	if err != nil || len(result.Steps) != 1 {
		t.Fatalf("specific repair did not recover: plan=%#v err=%v", result, err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected one repair call, got %d", len(provider.requests))
	}
	repair := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(repair, "summary must be a string") || !strings.Contains(repair, "Exact shape example") {
		t.Fatalf("repair prompt lost the actionable schema error: %s", repair)
	}
}

func TestSegmentPlannerReportsSpecificErrorAfterRepair(t *testing.T) {
	provider := mock.Provider{Response: `{"summary":7,"terminal_segment":true,"steps":[]}`}
	planner, _ := NewSegmentPlanner(provider)
	now := time.Now().UTC()
	horizonPlan := DefaultPlan("task", now)
	_, _, err := planner.Create(context.Background(), SegmentInput{
		DeploymentID: "dep",
		Task:         contracts.Task{ID: "task", Goal: "goal", Spec: contracts.TaskSpec{TaskID: "task", Budget: contracts.TaskBudget{MaxSteps: 20, MaxSegmentSteps: 4}}},
		Horizon:      contracts.HorizonState{HorizonID: horizonPlan.HorizonID, Plan: horizonPlan},
		Stage:        horizonPlan.Stages[0],
		Tools:        []contracts.ToolDefinition{{Name: "list_files", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}},
		Profile:      DefaultProfiles("dep", now)[0],
		Revision:     2,
	})
	if err == nil || !strings.Contains(err.Error(), "summary must be a string") {
		t.Fatalf("planner failure did not preserve the concrete decode error: %v", err)
	}
}

type plannerScriptProvider struct {
	responses []string
	requests  []contracts.ChatRequest
}

func (p *plannerScriptProvider) Chat(_ context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	p.requests = append(p.requests, request)
	response := p.responses[0]
	p.responses = p.responses[1:]
	return contracts.ChatResponse{Text: response}, nil
}

func (p *plannerScriptProvider) ChatStream(context.Context, contracts.ChatRequest, contracts.StreamSink) error {
	return nil
}

func (p *plannerScriptProvider) HealthCheck(context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true}
}

func (p *plannerScriptProvider) ProbeCapabilities(context.Context) contracts.CapabilitySnapshot {
	return contracts.CapabilitySnapshot{SupportsTools: true, ReliableContextTokens: 8192}
}
