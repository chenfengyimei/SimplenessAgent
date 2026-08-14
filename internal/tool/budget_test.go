package tool

import (
	"testing"
)

func TestAdjustBudget_RequiresPositiveInput(t *testing.T) {
	registry := NewRegistry()
	called := false
	if err := RegisterAdjustBudgetTool(registry, func(adj BudgetAdjustment) (int, int, error) {
		called = true
		return 0, 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := Invoke(registry, "adjust_context_budget")
	result, err := handler(nil, map[string]interface{}{"max_input_tokens": float64(0)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "FAILED" {
		t.Errorf("expected FAILED, got %s", result.Status)
	}
	if called {
		t.Error("callback should not be called for invalid input")
	}
}

func TestAdjustBudget_GrantsRequest(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterAdjustBudgetTool(registry, func(adj BudgetAdjustment) (int, int, error) {
		return adj.MaxInputTokens, adj.MaxOutputTokens, nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := Invoke(registry, "adjust_context_budget")
	result, err := handler(nil, map[string]interface{}{
		"max_input_tokens":  float64(32768),
		"max_output_tokens": float64(4096),
		"reason":            "need to read large files",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "SUCCEEDED" {
		t.Errorf("expected SUCCEEDED, got %s", result.Status)
	}
	if result.Data["granted_input_tokens"].(int) != 32768 {
		t.Errorf("expected 32768, got %v", result.Data["granted_input_tokens"])
	}
}

func TestAdjustBudget_CapsRequest(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterAdjustBudgetTool(registry, func(adj BudgetAdjustment) (int, int, error) {
		return 65536, 8192, nil // caps to 65536 regardless of request
	}); err != nil {
		t.Fatal(err)
	}
	handler := Invoke(registry, "adjust_context_budget")
	result, err := handler(nil, map[string]interface{}{
		"max_input_tokens": float64(999999),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "SUCCEEDED" {
		t.Errorf("expected SUCCEEDED, got %s", result.Status)
	}
	if result.Data["granted_input_tokens"].(int) != 65536 {
		t.Errorf("expected 65536, got %v", result.Data["granted_input_tokens"])
	}
	if result.Summary != "context budget partially granted (capped)" {
		t.Errorf("expected capped summary, got %q", result.Summary)
	}
}
