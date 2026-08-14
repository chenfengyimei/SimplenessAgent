package tool

import (
	"testing"
)

func TestAskUser_RequiresQuestion(t *testing.T) {
	registry := NewRegistry()
	called := false
	if err := RegisterAskUserTool(registry, func(q UserQuestion) error {
		called = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := Invoke(registry, "ask_user")
	result, err := handler(nil, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "FAILED" {
		t.Errorf("expected FAILED, got %s", result.Status)
	}
	if called {
		t.Error("callback should not be called for missing question")
	}
}

func TestAskUser_PassesOptions(t *testing.T) {
	registry := NewRegistry()
	var captured UserQuestion
	if err := RegisterAskUserTool(registry, func(q UserQuestion) error {
		captured = q
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	handler := Invoke(registry, "ask_user")
	result, err := handler(nil, map[string]interface{}{
		"question": "React or Vue?",
		"options":  []interface{}{"React", "Vue"},
		"context":  "Need to decide frontend framework",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "WAITING_USER" {
		t.Errorf("expected WAITING_USER, got %s", result.Status)
	}
	if captured.Question != "React or Vue?" {
		t.Errorf("expected question, got %q", captured.Question)
	}
	if len(captured.Options) != 2 {
		t.Errorf("expected 2 options, got %d", len(captured.Options))
	}
	if captured.Context != "Need to decide frontend framework" {
		t.Errorf("expected context, got %q", captured.Context)
	}
}
