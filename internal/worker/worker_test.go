package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xm/simplenessagent/internal/tool"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestRunGoldenReadOnlyToolLoop(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "lookup", ArgumentsJSON: `{"query":"agent"}`}}, Usage: contracts.TokenUsage{InputTokens: 4, OutputTokens: 3}},
		{Text: "Found the requested record.", Usage: contracts.TokenUsage{InputTokens: 6, OutputTokens: 2}},
	}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		if args["query"] != "agent" {
			t.Fatalf("unexpected arguments: %#v", args)
		}
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: "one record"}, nil
	})
	worker, err := New(provider, registry)
	if err != nil {
		t.Fatal(err)
	}
	contextPackage := &contracts.ContextPackage{Version: contracts.SchemaVersion, ID: "ctx_1", Role: "EXECUTOR", TaskID: "task_1", StepID: "step_1", CompilerVersion: "1.0.0", Budget: contracts.ContextBudget{Limit: 20, Used: 3, Reserved: 4}, Sections: []contracts.ContextSection{{Type: "TASK", Content: "find the record", SourceRefs: []string{"task_1"}, EstimatedTokens: 3}}}
	skill := contracts.Skill{Manifest: contracts.SkillManifest{Version: contracts.SchemaVersion, Name: "review", SkillVersion: "1.0.0", Description: "Review evidence", AllowedTools: []string{"lookup"}, WorkspaceScopes: []string{"."}}, Instructions: "Check evidence before responding."}
	result, err := worker.Run(context.Background(), Input{DeploymentID: "dep_1", Step: testStep(2), Context: "ignored", ContextPackage: contextPackage, Skills: []contracts.Skill{skill}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Found the requested record." || result.Iterations != 2 || result.Usage.InputTokens != 10 || len(result.ToolResults) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(provider.requests) != 2 || len(provider.requests[0].Tools) != 1 || provider.requests[0].Messages[0].Role != "system" || len(provider.requests[1].Messages) != 4 {
		t.Fatalf("worker did not build the controlled loop: %#v", provider.requests)
	}
	if !strings.Contains(provider.requests[0].Messages[1].Content, "[TASK] [sources: task_1]") || strings.Contains(provider.requests[0].Messages[1].Content, "ignored") {
		t.Fatalf("worker did not render bounded context package: %#v", provider.requests[0].Messages[1])
	}
	if !strings.Contains(provider.requests[0].Messages[1].Content, "[SKILL review]") {
		t.Fatalf("worker did not render selected skill: %#v", provider.requests[0].Messages[1])
	}
}

func TestRunExecutesParallelReadOnlyToolCallsInResponseOrder(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{
			{ID: "call_first", Name: "lookup", ArgumentsJSON: `{"query":"first"}`},
			{ID: "call_second", Name: "lookup", ArgumentsJSON: `{"query":"second"}`},
		}},
		{Text: "已完成两项检索。"},
	}}
	calls := []string{}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		calls = append(calls, args["query"].(string))
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: args["query"].(string)}, nil
	})
	executor, err := New(provider, registry)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Run(context.Background(), Input{Step: testStep(2)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "已完成两项检索。" || len(result.ToolResults) != 2 || strings.Join(calls, ",") != "first,second" {
		t.Fatalf("parallel tool calls were not safely completed: result=%#v calls=%#v", result, calls)
	}
	if len(provider.requests) != 2 || len(provider.requests[1].Messages) != 5 || provider.requests[1].Messages[3].ToolCallID != "call_first" || provider.requests[1].Messages[4].ToolCallID != "call_second" {
		t.Fatalf("tool result messages were not preserved in order: %#v", provider.requests)
	}
}

func TestRunRejectsSkillOutsideStepBoundary(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{Text: "not reached"}}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{}, nil
	})
	worker, _ := New(provider, registry)
	contextPackage := &contracts.ContextPackage{Version: contracts.SchemaVersion, ID: "ctx", Role: "EXECUTOR", TaskID: "task", StepID: "step_1", CompilerVersion: "1.0.0", Budget: contracts.ContextBudget{Limit: 20, Used: 0}}
	outside := contracts.Skill{Manifest: contracts.SkillManifest{Version: contracts.SchemaVersion, Name: "writer", SkillVersion: "1.0.0", Description: "Write", AllowedTools: []string{"write_file"}, WorkspaceScopes: []string{"."}}, Instructions: "write"}
	_, err := worker.Run(context.Background(), Input{Step: testStep(1), ContextPackage: contextPackage, Skills: []contracts.Skill{outside}})
	assertCode(t, err, contracts.ErrToolNotAllowed)
	if len(provider.requests) != 0 {
		t.Fatal("invalid skill must not reach provider")
	}
}

func TestRunRejectsInvalidContextPackage(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{Text: "not reached"}}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{}, nil
	})
	worker, _ := New(provider, registry)
	invalid := &contracts.ContextPackage{Version: contracts.SchemaVersion, ID: "ctx", Role: "EXECUTOR", TaskID: "task", StepID: "another-step", CompilerVersion: "1.0.0", Budget: contracts.ContextBudget{Limit: 10, Used: 0}}
	_, err := worker.Run(context.Background(), Input{Step: testStep(1), ContextPackage: invalid})
	assertCode(t, err, contracts.ErrInvalidInput)
	if len(provider.requests) != 0 {
		t.Fatal("invalid context must not reach provider")
	}
}

func TestRunRejectsToolOutsideStepAllowlist(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "other", ArgumentsJSON: `{}`}}}}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		t.Fatal("tool must not run")
		return contracts.ToolResult{}, nil
	})
	if err := registry.Register(contracts.ToolDefinition{Name: "other", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}, func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		t.Fatal("unallowed tool must not run")
		return contracts.ToolResult{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	worker, _ := New(provider, registry)
	_, err := worker.Run(context.Background(), Input{Step: testStep(1)})
	assertCode(t, err, contracts.ErrToolNotAllowed)
}

func TestRunRejectsInvalidOrRepeatedToolActions(t *testing.T) {
	t.Run("invalid schema with retry", func(t *testing.T) {
		invalidResponse := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "lookup", ArgumentsJSON: `{}`}}}
		validResponse := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{{ID: "call_2", Name: "lookup", ArgumentsJSON: `{"query":"fixed"}`}}}
		textResponse := contracts.ChatResponse{Text: "Done after retry."}
		provider := &scriptedProvider{responses: []contracts.ChatResponse{invalidResponse, validResponse, textResponse}}
		registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
			return contracts.ToolResult{Status: "SUCCEEDED", Summary: "recovered after retry"}, nil
		})
		worker, _ := New(provider, registry)
		result, err := worker.Run(context.Background(), Input{Step: testStep(3)})
		if err != nil {
			t.Fatalf("expected retry to succeed, got error: %v", err)
		}
		if len(result.ToolResults) != 1 || result.ToolResults[0].Summary != "recovered after retry" {
			t.Fatalf("unexpected results: %#v", result)
		}
	})
	t.Run("invalid schema exhausts retry", func(t *testing.T) {
		invalidResponse := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "lookup", ArgumentsJSON: `{}`}}}
		provider := &scriptedProvider{responses: []contracts.ChatResponse{invalidResponse, invalidResponse}}
		registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
			t.Fatal("invalid tool must not run")
			return contracts.ToolResult{}, nil
		})
		worker, _ := New(provider, registry)
		_, err := worker.Run(context.Background(), Input{Step: testStep(3)})
		assertCode(t, err, contracts.ErrInvalidToolCall)
	})
	t.Run("repeated action", func(t *testing.T) {
		response := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "lookup", ArgumentsJSON: `{"query":"same"}`}}}
		provider := &scriptedProvider{responses: []contracts.ChatResponse{response, response}}
		calls := 0
		registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
			calls++
			return contracts.ToolResult{Status: "SUCCEEDED"}, nil
		})
		worker, _ := New(provider, registry)
		result, err := worker.Run(context.Background(), Input{Step: testStep(2)})
		assertCode(t, err, contracts.ErrRepeatedAction)
		if calls != 1 || len(result.ToolResults) != 1 {
			t.Fatalf("repeated action was invoked: %#v", result)
		}
	})
}

func TestRunStopsAtBudgetAndRejectsWriteTools(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "lookup", ArgumentsJSON: `{"query":"one"}`}}}}}
	registry := testRegistry(t, contracts.RiskWrite, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		t.Fatal("write tool must not run")
		return contracts.ToolResult{}, nil
	})
	worker, _ := New(provider, registry)
	_, err := worker.Run(context.Background(), Input{Step: testStep(1)})
	assertCode(t, err, contracts.ErrToolNotAllowed)

	provider = &scriptedProvider{responses: []contracts.ChatResponse{{Text: "too many tokens", Usage: contracts.TokenUsage{OutputTokens: 3}}}}
	registry = testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{}, nil
	})
	worker, _ = New(provider, registry)
	step := testStep(1)
	step.Budget.MaxOutputTokens = 2
	_, err = worker.Run(context.Background(), Input{Step: step})
	assertCode(t, err, contracts.ErrBudgetExceeded)

	provider = &scriptedProvider{delay: 10 * time.Millisecond, responses: []contracts.ChatResponse{{Text: "late"}}}
	worker, _ = New(provider, registry)
	step = testStep(1)
	step.Budget.MaxDurationMS = 1
	_, err = worker.Run(context.Background(), Input{Step: step})
	assertCode(t, err, contracts.ErrRequestTimeout)
}

func TestRunStopsAfterApprovalGatedWriteProposal(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "proposal", Name: "propose_write_file", ArgumentsJSON: `{}`}}}}}
	registry := tool.NewRegistry()
	if err := registry.Register(contracts.ToolDefinition{Name: "propose_write_file", RiskClass: contracts.RiskWrite, ParametersSchema: map[string]interface{}{"type": "object", "additionalProperties": false}}, func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{Status: "WAITING_APPROVAL", Summary: "waiting"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	executor, err := New(provider, registry)
	if err != nil {
		t.Fatal(err)
	}
	step := testStep(2)
	step.Risk = contracts.RiskWrite
	step.AllowedTools = []string{"propose_write_file"}
	result, err := executor.Run(context.Background(), Input{Step: step})
	if err != nil || len(result.ToolResults) != 1 || result.ToolResults[0].Status != "WAITING_APPROVAL" || len(provider.requests) != 1 {
		t.Fatalf("proposal should pause the loop without a direct write: result=%#v err=%v requests=%#v", result, err, provider.requests)
	}
}

func testRegistry(t *testing.T, risk contracts.RiskClass, schema map[string]interface{}, handler tool.Handler) *tool.Registry {
	t.Helper()
	registry := tool.NewRegistry()
	if err := registry.Register(contracts.ToolDefinition{Name: "lookup", RiskClass: risk, ParametersSchema: schema}, handler); err != nil {
		t.Fatal(err)
	}
	return registry
}
func strictQuerySchema() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}}, "required": []interface{}{"query"}, "additionalProperties": false}
}
func testStep(maxIterations int) contracts.StepSpec {
	return contracts.StepSpec{StepID: "step_1", Title: "Lookup", Goal: "Find a record", AllowedTools: []string{"lookup"}, WorkspaceScopes: []string{"."}, Budget: contracts.StepBudget{MaxIterations: maxIterations}}
}
func assertCode(t *testing.T, err error, wanted contracts.ErrorCode) {
	t.Helper()
	var domain *contracts.Error
	if !errors.As(err, &domain) || domain.Code != wanted {
		t.Fatalf("wanted %s, got %#v", wanted, err)
	}
}

type scriptedProvider struct {
	responses []contracts.ChatResponse
	requests  []contracts.ChatRequest
	delay     time.Duration
}

func (p *scriptedProvider) Chat(_ context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	p.requests = append(p.requests, request)
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if len(p.responses) == 0 {
		return contracts.ChatResponse{}, errors.New("unexpected chat call")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}
func (p *scriptedProvider) ChatStream(_ context.Context, _ contracts.ChatRequest, _ contracts.StreamSink) error {
	return errors.New("streaming is not used")
}
func (p *scriptedProvider) HealthCheck(_ context.Context) contracts.HealthStatus {
	return contracts.HealthStatus{Healthy: true}
}
func (p *scriptedProvider) ProbeCapabilities(_ context.Context) contracts.CapabilitySnapshot {
	return contracts.CapabilitySnapshot{ProbedAt: time.Now()}
}
