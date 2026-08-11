package planner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestCreateAcceptsOnlyValidatedReadOnlyPlan(t *testing.T) {
	provider := &fakeProvider{response: `{"version":1,"plan_id":"untrusted","task_id":"untrusted","revision":99,"reason":"INITIAL","summary":"inspect","steps":[{"version":1,"step_id":"inspect","title":"Inspect","goal":"Read files","allowed_tools":["list_files"],"workspace_scopes":["."],"acceptance_criteria":[{"id":"evidence","type":"EVIDENCE_EXISTS","description":"report","spec":{}}],"risk":"READ","budget":{"max_attempts":1,"max_iterations":2,"max_duration_ms":1000,"max_input_tokens":10,"max_output_tokens":10}}]}`}
	planner, err := New(provider)
	if err != nil {
		t.Fatal(err)
	}
	created, err := planner.Create(context.Background(), Input{DeploymentID: "dep", Task: testTask(), AvailableTools: []contracts.ToolDefinition{readTool()}, Revision: 2, ParentPlanID: "pln_parent"})
	if err != nil {
		t.Fatal(err)
	}
	if created.TaskID != "tsk_1" || created.Revision != 2 || created.ParentPlanID != "pln_parent" || created.PlanID == "untrusted" || created.CreatedByAgent != "model-planner" {
		t.Fatalf("trusted plan metadata not applied: %#v", created)
	}
	if len(provider.requests) != 1 || len(provider.requests[0].Tools) != 0 || provider.requests[0].Messages[0].Role != "system" {
		t.Fatalf("planner request is not read-only: %#v", provider.requests)
	}
}

func TestCreateRejectsInvalidPlanAndWriteTool(t *testing.T) {
	t.Run("not JSON", func(t *testing.T) {
		planner, _ := New(&fakeProvider{response: "```json\n{}\n```"})
		_, err := planner.Create(context.Background(), Input{Task: testTask(), AvailableTools: []contracts.ToolDefinition{readTool()}})
		assertPlanError(t, err)
	})
	t.Run("tool outside allowlist", func(t *testing.T) {
		response := `{"steps":[{"version":1,"step_id":"write","title":"Write","goal":"Change","allowed_tools":["write_file"],"workspace_scopes":["."],"acceptance_criteria":[{"id":"e","type":"EVIDENCE_EXISTS","description":"x","spec":{}}],"risk":"WRITE","budget":{"max_attempts":1,"max_iterations":1,"max_duration_ms":1}}]}`
		planner, _ := New(&fakeProvider{response: response})
		_, err := planner.Create(context.Background(), Input{Task: testTask(), AvailableTools: []contracts.ToolDefinition{readTool()}})
		assertPlanError(t, err)
	})
}

func TestCreateLocalReplanUsesNewStepIDs(t *testing.T) {
	previous := contracts.PlanVersion{PlanID: "pln_previous", Steps: []contracts.StepSpec{{StepID: "inspect"}}}
	response := `{"summary":"replan","steps":[{"version":1,"step_id":"inspect","title":"Inspect","goal":"Read files","allowed_tools":["list_files"],"workspace_scopes":["."],"acceptance_criteria":[{"id":"evidence","type":"EVIDENCE_EXISTS","description":"report","spec":{}}],"risk":"READ","budget":{"max_attempts":1,"max_iterations":2,"max_duration_ms":1000,"max_input_tokens":10,"max_output_tokens":10}}]}`
	planner, err := New(&fakeProvider{response: response})
	if err != nil {
		t.Fatal(err)
	}
	_, err = planner.Create(context.Background(), Input{DeploymentID: "dep", Task: testTask(), AvailableTools: []contracts.ToolDefinition{readTool()}, Revision: 2, ParentPlanID: previous.PlanID, ReplanContext: &ReplanContext{Reason: "interrupted", PreviousPlan: previous}})
	assertPlanError(t, err)
}

func TestCreateLocalReplanNormalizesReason(t *testing.T) {
	previous := contracts.PlanVersion{PlanID: "pln_previous", Steps: []contracts.StepSpec{{StepID: "old"}}}
	response := `{"summary":"replan","reason":"UNTRUSTED","steps":[{"version":1,"step_id":"new","title":"Inspect","goal":"Read files","allowed_tools":["list_files"],"workspace_scopes":["."],"acceptance_criteria":[{"id":"evidence","type":"EVIDENCE_EXISTS","description":"report","spec":{}}],"risk":"READ","budget":{"max_attempts":1,"max_iterations":2,"max_duration_ms":1000,"max_input_tokens":10,"max_output_tokens":10}}]}`
	planner, _ := New(&fakeProvider{response: response})
	created, err := planner.Create(context.Background(), Input{DeploymentID: "dep", Task: testTask(), AvailableTools: []contracts.ToolDefinition{readTool()}, Revision: 2, ParentPlanID: previous.PlanID, ReplanContext: &ReplanContext{Reason: "interrupted", PreviousPlan: previous}})
	if err != nil || created.Reason != "LOCAL_REPLAN" {
		t.Fatal(created, err)
	}
}

func readTool() contracts.ToolDefinition {
	return contracts.ToolDefinition{Name: "list_files", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}
}
func testTask() contracts.Task {
	spec := contracts.TaskSpec{Version: contracts.SchemaVersion, TaskID: "tsk_1", Budget: contracts.TaskBudget{MaxSteps: 2, MaxDurationMS: 10000, MaxModelInputTokens: 100, MaxModelOutputTokens: 100}}
	return contracts.Task{ID: "tsk_1", Version: contracts.SchemaVersion, Spec: spec}
}
func assertPlanError(t *testing.T, err error) {
	t.Helper()
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != contracts.ErrPlanInvalid {
		t.Fatalf("expected PLAN_INVALID, got %#v", err)
	}
}

type fakeProvider struct {
	response string
	requests []contracts.ChatRequest
}

func (p *fakeProvider) Chat(_ context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	p.requests = append(p.requests, request)
	return contracts.ChatResponse{Text: p.response}, nil
}
func (p *fakeProvider) ChatStream(context.Context, contracts.ChatRequest, contracts.StreamSink) error {
	return errors.New("not used")
}
func (p *fakeProvider) HealthCheck(context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true}
}
func (p *fakeProvider) ProbeCapabilities(context.Context) contracts.CapabilitySnapshot {
	return contracts.CapabilitySnapshot{ProbedAt: time.Now()}
}
