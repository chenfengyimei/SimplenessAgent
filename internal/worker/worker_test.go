package worker

import (
	"context"
	"encoding/json"
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

func TestWriteStepCannotFinishWithoutWriteProposal(t *testing.T) {
	t.Run("recovers via obligation instruction", func(t *testing.T) {
		provider := &scriptedProvider{responses: []contracts.ChatResponse{
			{Text: "I inspected everything; no writes needed."},
			{ToolCalls: []contracts.ToolCall{{ID: "write_1", Name: "write_file", ArgumentsJSON: `{"path":"index.html","content":"<html></html>"}`}}},
		}, rejectOrphanToolCalls: true}
		registry := tool.NewRegistry()
		if err := registry.Register(contracts.ToolDefinition{Name: "write_file", RiskClass: contracts.RiskWrite, ParametersSchema: map[string]interface{}{"type": "object"}}, func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
			return contracts.ToolResult{Status: "WAITING_APPROVAL", Summary: "waiting for user approval"}, nil
		}); err != nil {
			t.Fatal(err)
		}
		worker, _ := New(provider, registry)
		step := contracts.StepSpec{StepID: "step_1", Title: "Write", Goal: "Create index.html", AllowedTools: []string{"write_file"}, WorkspaceScopes: []string{"."}, Risk: contracts.RiskWrite, Budget: contracts.StepBudget{MaxIterations: 3}}
		result, err := worker.Run(context.Background(), Input{Step: step, WriteCompletionRequired: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.ToolResults) != 1 || result.ToolResults[0].Status != "WAITING_APPROVAL" {
			t.Fatalf("write obligation repair did not lead to a proposal: %#v", result)
		}
		instruction := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
		if instruction.Role != "user" || !strings.Contains(instruction.Content, "no write tool has been used yet") {
			t.Fatalf("obligation instruction missing: %#v", instruction)
		}
	})
	t.Run("fails closed after ignoring the instruction", func(t *testing.T) {
		provider := &scriptedProvider{responses: []contracts.ChatResponse{
			{Text: "reads are enough."},
			{Text: "still no writes."},
		}}
		registry := tool.NewRegistry()
		if err := registry.Register(contracts.ToolDefinition{Name: "write_file", RiskClass: contracts.RiskWrite, ParametersSchema: map[string]interface{}{"type": "object"}}, func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
			t.Fatal("write tool must not run")
			return contracts.ToolResult{}, nil
		}); err != nil {
			t.Fatal(err)
		}
		worker, _ := New(provider, registry)
		step := contracts.StepSpec{StepID: "step_1", Title: "Write", Goal: "Create index.html", AllowedTools: []string{"write_file"}, WorkspaceScopes: []string{"."}, Risk: contracts.RiskWrite, Budget: contracts.StepBudget{MaxIterations: 2}}
		_, err := worker.Run(context.Background(), Input{Step: step, WriteCompletionRequired: true})
		if err == nil || !strings.Contains(err.Error(), "write-intent step ended without any write proposal") {
			t.Fatalf("write step completed without a proposal: %v", err)
		}
	})
	t.Run("read-only steps are unaffected", func(t *testing.T) {
		provider := &scriptedProvider{responses: []contracts.ChatResponse{{Text: "Evidence-based summary."}}}
		registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
			return contracts.ToolResult{Status: "SUCCEEDED"}, nil
		})
		worker, _ := New(provider, registry)
		result, err := worker.Run(context.Background(), Input{Step: testStep(1)})
		if err != nil || result.Text != "Evidence-based summary." {
			t.Fatalf("read-only step was blocked: result=%#v err=%v", result, err)
		}
	})
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
	t.Run("repeated read is served from cache", func(t *testing.T) {
		response := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "lookup", ArgumentsJSON: `{"query":"same"}`}}}
		provider := &scriptedProvider{responses: []contracts.ChatResponse{response, response, {Text: "Used the replayed evidence."}}}
		calls := 0
		registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
			calls++
			return contracts.ToolResult{Status: "SUCCEEDED"}, nil
		})
		worker, _ := New(provider, registry)
		result, err := worker.Run(context.Background(), Input{Step: testStep(3)})
		if err != nil {
			t.Fatal(err)
		}
		if calls != 1 || len(result.ToolResults) != 2 || result.Text != "Used the replayed evidence." {
			t.Fatalf("repeated read should replay the cached result: calls=%d result=%#v", calls, result)
		}
	})
	t.Run("repeated mutating action stays blocked", func(t *testing.T) {
		response := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "write_file", ArgumentsJSON: `{"path":"a","content":"x"}`}}}
		provider := &scriptedProvider{responses: []contracts.ChatResponse{response, response, response}}
		calls := 0
		registry := tool.NewRegistry()
		if err := registry.Register(contracts.ToolDefinition{Name: "write_file", RiskClass: contracts.RiskWrite, ParametersSchema: map[string]interface{}{"type": "object"}}, func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
			calls++
			return contracts.ToolResult{Status: "SUCCEEDED"}, nil
		}); err != nil {
			t.Fatal(err)
		}
		worker, _ := New(provider, registry)
		step := contracts.StepSpec{StepID: "step_1", Title: "Write", Goal: "Persist change", AllowedTools: []string{"write_file"}, WorkspaceScopes: []string{"."}, Budget: contracts.StepBudget{MaxIterations: 3}}
		_, err := worker.Run(context.Background(), Input{Step: step})
		assertCode(t, err, contracts.ErrRepeatedAction)
		if calls != 1 {
			t.Fatalf("mutating tool executed more than once: %d", calls)
		}
	})
}

func TestRunRepairsTooManyToolsBeforeAnyInvocation(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{
			{ID: "too_many_1", Name: "lookup", ArgumentsJSON: `{"query":"first"}`},
			{ID: "too_many_2", Name: "lookup", ArgumentsJSON: `{"query":"second"}`},
		}},
		{ToolCalls: []contracts.ToolCall{{ID: "repaired", Name: "lookup", ArgumentsJSON: `{"query":"only"}`}}},
		{Text: "Done after bounded retry."},
	}, rejectOrphanToolCalls: true}
	invocations := 0
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		invocations++
		if args["query"] != "only" {
			t.Fatalf("an over-limit tool call was invoked: %#v", args)
		}
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: "bounded"}, nil
	})
	executor, _ := New(provider, registry)
	result, err := executor.Run(context.Background(), Input{Step: testStep(3), MaxToolCallsPerResponse: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Done after bounded retry." || invocations != 1 || len(result.ToolResults) != 1 || result.Iterations != 3 {
		t.Fatalf("tool-limit repair was not side-effect safe: result=%#v invocations=%d", result, invocations)
	}
	if len(provider.requests) != 3 {
		t.Fatalf("expected one repair and one evidence response, got %d requests", len(provider.requests))
	}
	repair := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(repair, "requested 2 tools") || !strings.Contains(repair, "limit is 1") {
		t.Fatalf("repair prompt did not explain the tool-call limit: %s", repair)
	}
	for _, message := range provider.requests[1].Messages {
		if len(message.ToolCalls) != 0 {
			t.Fatalf("rejected tool calls were replayed without tool results: %#v", provider.requests[1].Messages)
		}
	}
}

func TestRunRepairNamesAllowedTools(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{{ID: "forbidden", Name: "other", ArgumentsJSON: `{}`}}},
		{ToolCalls: []contracts.ToolCall{{ID: "allowed", Name: "lookup", ArgumentsJSON: `{"query":"ok"}`}}},
		{Text: "Recovered inside the allowlist."},
	}, rejectOrphanToolCalls: true}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: "ok"}, nil
	})
	if err := registry.Register(contracts.ToolDefinition{Name: "other", RiskClass: contracts.RiskRead, ParametersSchema: map[string]interface{}{"type": "object"}}, func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		t.Fatal("unallowed tool must not run")
		return contracts.ToolResult{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	worker, _ := New(provider, registry)
	result, err := worker.Run(context.Background(), Input{Step: testStep(3), MaxToolCallsPerResponse: 1})
	if err != nil || result.Text != "Recovered inside the allowlist." {
		t.Fatalf("allowlist repair did not recover: result=%#v err=%v", result, err)
	}
	if len(provider.requests) != 3 {
		t.Fatalf("expected exactly one repair round, got %d requests", len(provider.requests))
	}
	repair := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(repair, "ONLY tools allowed for this step are: lookup") || !strings.Contains(repair, "at most 1 tool call(s)") {
		t.Fatalf("repair prompt did not name the allowlist and limit: %s", repair)
	}
}

func TestRunPrevalidatesEntireToolBatchBeforeInvocation(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{
			{ID: "valid_but_not_executed", Name: "lookup", ArgumentsJSON: `{"query":"must-not-run"}`},
			{ID: "invalid", Name: "lookup", ArgumentsJSON: `{}`},
		}},
		{ToolCalls: []contracts.ToolCall{{ID: "fixed", Name: "lookup", ArgumentsJSON: `{"query":"fixed"}`}}},
		{Text: "Validated before execution."},
	}, rejectOrphanToolCalls: true}
	queries := []string{}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		queries = append(queries, args["query"].(string))
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: "validated"}, nil
	})
	executor, _ := New(provider, registry)
	result, err := executor.Run(context.Background(), Input{Step: testStep(3), MaxToolCallsPerResponse: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Validated before execution." || strings.Join(queries, ",") != "fixed" || len(result.ToolResults) != 1 {
		t.Fatalf("a partially invalid batch caused an invocation: result=%#v queries=%#v", result, queries)
	}
}

func TestRunRepeatedReadIsServedFromCacheAsToolResponse(t *testing.T) {
	repeated := contracts.ChatResponse{ToolCalls: []contracts.ToolCall{{ID: "read-again", Name: "lookup", ArgumentsJSON: `{"query":"same"}`}}}
	provider := &scriptedProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{{ID: "read-once", Name: "lookup", ArgumentsJSON: `{"query":"same"}`}}},
		repeated,
		{Text: "Used the replayed evidence."},
	}, rejectOrphanToolCalls: true}
	invocations := 0
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		invocations++
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: "existing evidence"}, nil
	})
	executor, _ := New(provider, registry)
	result, err := executor.Run(context.Background(), Input{Step: testStep(3), MaxToolCallsPerResponse: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "Used the replayed evidence." || invocations != 1 || len(result.ToolResults) != 2 || result.Iterations != 3 {
		t.Fatalf("repeated read was re-executed instead of replayed from cache: result=%#v invocations=%d", result, invocations)
	}
	second := provider.requests[2].Messages
	var replayMessage *contracts.Message
	for index := range second {
		if second[index].Role == "tool" && second[index].ToolCallID == "read-again" {
			replayMessage = &second[index]
		}
	}
	if replayMessage == nil || !strings.Contains(replayMessage.Content, "existing evidence") {
		t.Fatalf("replay did not return the cached result as a protocol tool response: %#v", second)
	}
}

func TestRunFinalizesSuccessfulReadEvidenceOnLastIteration(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{
		{ToolCalls: []contracts.ToolCall{{ID: "read-first", Name: "lookup", ArgumentsJSON: `{"query":"first"}`}}},
		{ToolCalls: []contracts.ToolCall{{ID: "read-final", Name: "lookup", ArgumentsJSON: `{"query":"final"}`}}},
	}}
	queries := []string{}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		queries = append(queries, args["query"].(string))
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: "fresh evidence"}, nil
	})
	executor, _ := New(provider, registry)
	step := testStep(2)
	step.Title = "搜索相关文件"
	result, err := executor.Run(context.Background(), Input{Step: step})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(queries, ",") != "first,final" || len(result.ToolResults) != 2 || result.Iterations != 2 || !strings.Contains(result.Text, "最后允许回合") {
		t.Fatalf("last-turn read evidence was not finalized deterministically: result=%#v queries=%#v", result, queries)
	}
}

func TestRunDoesNotFinalizeFailedReadOnLastIteration(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "failed-read", Name: "lookup", ArgumentsJSON: `{"query":"missing"}`}}}}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{Status: "FAILED", Summary: "temporary failure", Error: &contracts.ToolError{Code: string(contracts.ErrProviderInternal), Message: "temporary failure", Retryable: true}}, nil
	})
	executor, _ := New(provider, registry)
	result, err := executor.Run(context.Background(), Input{Step: testStep(1)})
	assertCode(t, err, contracts.ErrBudgetExceeded)
	if len(result.ToolResults) != 1 || result.ToolResults[0].Status != "FAILED" || result.Text != "" {
		t.Fatalf("retryable failed read must remain fail-closed: %#v", result)
	}
}

func TestRunFinalizesStableNegativeReadOutcomeOnLastIteration(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{ToolCalls: []contracts.ToolCall{{ID: "negative-read", Name: "lookup", ArgumentsJSON: `{"query":"git"}`}}}}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{Status: "FAILED", Summary: "not a Git repository", Error: &contracts.ToolError{Code: string(contracts.ErrInvalidInput), Message: "not a Git repository"}}, nil
	})
	executor, _ := New(provider, registry)
	step := testStep(1)
	step.Goal = "检查工作区"
	result, err := executor.Run(context.Background(), Input{Step: step})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolResults) != 1 || result.ToolResults[0].Status != "FAILED" || !strings.Contains(result.Text, "不可重试的负面结果") {
		t.Fatalf("stable negative read outcome was not preserved as evidence: %#v", result)
	}
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

func TestRunPreflightsContextAndDoesNotRejectProviderPromptAccounting(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{Text: "brief answer", Usage: contracts.TokenUsage{InputTokens: 12000, OutputTokens: 12}}}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{}, nil
	})
	executor, err := New(provider, registry)
	if err != nil {
		t.Fatal(err)
	}
	step := testStep(1)
	step.Budget.MaxInputTokens = 64 // Context-package budget, not provider replay accounting.
	step.Budget.MaxOutputTokens = 128
	result, err := executor.Run(context.Background(), Input{Step: step, ReliableContextTokens: 4096})
	if err != nil || result.Text != "brief answer" {
		t.Fatalf("a bounded response must not fail solely because the provider reports the complete prompt: result=%#v err=%v", result, err)
	}
	if len(provider.requests) != 1 || provider.requests[0].MaxOutputTokens != 128 {
		t.Fatalf("worker did not send the response ceiling to the provider: %#v", provider.requests)
	}
}

func TestRunRejectsOverfullPromptBeforeCallingProvider(t *testing.T) {
	provider := &scriptedProvider{responses: []contracts.ChatResponse{{Text: "must not be used"}}}
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(context.Context, map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{}, nil
	})
	executor, _ := New(provider, registry)
	step := testStep(1)
	contextPackage := &contracts.ContextPackage{Version: contracts.SchemaVersion, ID: "ctx", Role: "EXECUTOR", TaskID: "task", StepID: step.StepID, CompilerVersion: "1.0.0", Budget: contracts.ContextBudget{Limit: 2000, Used: 1000}, Sections: []contracts.ContextSection{{Type: "TASK", Content: strings.Repeat("x", 4000), EstimatedTokens: 1000}}}
	_, err := executor.Run(context.Background(), Input{Step: step, ContextPackage: contextPackage, ReliableContextTokens: 512})
	assertCode(t, err, contracts.ErrContextOverflow)
	if len(provider.requests) != 0 {
		t.Fatalf("overfull prompts must be rejected locally: %#v", provider.requests)
	}
}

func TestRunRetriesTransientProviderWithoutReplayingToolSideEffect(t *testing.T) {
	provider := &scriptedProvider{
		chatErrors: []error{nil, contracts.NewError(contracts.ErrRateLimited, "busy"), nil},
		responses: []contracts.ChatResponse{
			{ToolCalls: []contracts.ToolCall{{ID: "call_1", Name: "lookup", ArgumentsJSON: `{"query":"once"}`}}},
			{Text: "done"},
		},
	}
	invocations := 0
	registry := testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, _ map[string]interface{}) (contracts.ToolResult, error) {
		invocations++
		return contracts.ToolResult{Status: "SUCCEEDED", Summary: "once"}, nil
	})
	executor, err := New(provider, registry)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Run(context.Background(), Input{DeploymentID: "dep", Step: testStep(2)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "done" || invocations != 1 || len(provider.requests) != 3 {
		t.Fatalf("retry replayed work or lost response: result=%#v invocations=%d requests=%d", result, invocations, len(provider.requests))
	}
}

func TestRunRecompilesOnceAfterProviderContextOverflow(t *testing.T) {
	provider := &scriptedProvider{
		chatErrors: []error{contracts.NewError(contracts.ErrContextOverflow, "provider rejected prompt"), nil},
		responses:  []contracts.ChatResponse{{Text: "recovered"}},
	}
	executor, err := New(provider, testRegistry(t, contracts.RiskRead, strictQuerySchema(), func(_ context.Context, _ map[string]interface{}) (contracts.ToolResult, error) {
		return contracts.ToolResult{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	contextText := strings.Repeat("long context data ", 200)
	result, err := executor.Run(context.Background(), Input{DeploymentID: "dep", Step: testStep(1), Context: contextText, ReliableContextTokens: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || len(provider.requests) != 2 {
		t.Fatalf("expected one compacted retry, got result=%#v requests=%d", result, len(provider.requests))
	}
	first := provider.requests[0].Messages[1].Content
	second := provider.requests[1].Messages[1].Content
	if len(second) >= len(first) || !strings.Contains(second, "context recompiled after provider overflow") {
		t.Fatalf("overflow retry was not traceably compacted: first=%d second=%d", len(first), len(second))
	}
}

func TestMarshalToolResultForModelKeepsBoundedValidJSON(t *testing.T) {
	encoded, err := marshalToolResultForModel(contracts.ToolResult{Status: "SUCCEEDED", Summary: "file read", Data: map[string]interface{}{"content": strings.Repeat("x", 10000)}}, 128)
	if err != nil || !json.Valid(encoded) || !strings.Contains(string(encoded), "data_truncated") {
		t.Fatalf("large tool output was not compacted safely: %s, %v", encoded, err)
	}
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
	responses             []contracts.ChatResponse
	chatErrors            []error
	requests              []contracts.ChatRequest
	delay                 time.Duration
	rejectOrphanToolCalls bool
}

func (p *scriptedProvider) Chat(_ context.Context, request contracts.ChatRequest) (contracts.ChatResponse, error) {
	p.requests = append(p.requests, request)
	if p.rejectOrphanToolCalls {
		for index, message := range request.Messages {
			if len(message.ToolCalls) == 0 {
				continue
			}
			pending := map[string]bool{}
			for _, call := range message.ToolCalls {
				pending[call.ID] = true
			}
			for next := index + 1; next < len(request.Messages) && request.Messages[next].Role == "tool"; next++ {
				delete(pending, request.Messages[next].ToolCallID)
			}
			if len(pending) != 0 {
				return contracts.ChatResponse{}, contracts.NewError(contracts.ErrInvalidInput, "assistant tool_calls must be followed by matching tool messages")
			}
		}
	}
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	if len(p.chatErrors) > 0 {
		err := p.chatErrors[0]
		p.chatErrors = p.chatErrors[1:]
		if err != nil {
			return contracts.ChatResponse{}, err
		}
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
