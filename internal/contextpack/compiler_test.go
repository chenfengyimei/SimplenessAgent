package contextpack

import (
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestCompileAppliesBudgetPriorityAndSourceDiversity(t *testing.T) {
	result, err := Compile(Input{Role: "EXECUTOR", TaskID: "tsk", BudgetLimit: 10, ReservedTokens: 2, Sections: []contracts.ContextSection{
		{Type: "TASK", Content: "high", SourceRefs: []string{"task"}, EstimatedTokens: 5, Priority: 100},
		{Type: "EVENT", Content: "same source", SourceRefs: []string{"task"}, EstimatedTokens: 2, Priority: 90},
		{Type: "EVIDENCE", Content: "other", SourceRefs: []string{"evidence"}, EstimatedTokens: 3, Priority: 80},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Package.Budget.Used != 8 || len(result.Package.Sections) != 2 || len(result.Omitted) != 1 {
		t.Fatalf("unexpected compilation: %#v", result)
	}
	if result.Package.Sections[0].Type != "TASK" || result.Package.Sections[1].Type != "EVIDENCE" {
		t.Fatalf("unexpected selected sections: %#v", result.Package.Sections)
	}
}

func TestCompileFailsClosedWhenNoSectionFits(t *testing.T) {
	_, err := Compile(Input{Role: "EXECUTOR", TaskID: "tsk", BudgetLimit: 5, ReservedTokens: 1, Sections: []contracts.ContextSection{{Type: "TASK", Content: "large", EstimatedTokens: 5}}})
	if domain, ok := err.(*contracts.Error); !ok || domain.Code != contracts.ErrContextOverflow {
		t.Fatalf("expected CONTEXT_OVERFLOW, got %#v", err)
	}
}

func TestCompileEstimatesAndNormalizesSources(t *testing.T) {
	result, err := Compile(Input{Role: "PLANNER", TaskID: "tsk", BudgetLimit: 10, Sections: []contracts.ContextSection{{Type: "TASK", Content: "12345", SourceRefs: []string{"b", "a", "a"}}}})
	if err != nil {
		t.Fatal(err)
	}
	section := result.Package.Sections[0]
	if section.EstimatedTokens != 2 || len(section.SourceRefs) != 2 || section.SourceRefs[0] != "a" || result.Package.CompilerVersion != compilerVersion {
		t.Fatalf("unexpected normalized section: %#v", result.Package)
	}
}
