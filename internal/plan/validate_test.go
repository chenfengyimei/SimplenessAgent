package plan

import (
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestValidateRejectsDependencyCycle(t *testing.T) {
	candidate := contracts.PlanVersion{Version: contracts.SchemaVersion, PlanID: "pln_test", TaskID: "tsk_test", Steps: []contracts.StepSpec{
		{Version: 1, StepID: "a", Title: "a", Goal: "a", Dependencies: []string{"b"}, AllowedTools: []string{"read_file"}, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "a", Type: contracts.AcceptanceEvidenceExists}}},
		{Version: 1, StepID: "b", Title: "b", Goal: "b", Dependencies: []string{"a"}, AllowedTools: []string{"read_file"}, AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "b", Type: contracts.AcceptanceEvidenceExists}}},
	}}
	result := Validate(candidate, 4)
	if result.Valid() {
		t.Fatalf("cyclic plan was accepted")
	}
}

func TestReadyStepsWaitsForDependencies(t *testing.T) {
	candidate := contracts.PlanVersion{Steps: []contracts.StepSpec{{StepID: "inspect"}, {StepID: "verify", Dependencies: []string{"inspect"}}}}
	ready := ReadySteps(candidate, map[string]contracts.StepStatus{"inspect": contracts.StepPending, "verify": contracts.StepPending})
	if len(ready) != 1 || ready[0] != "inspect" {
		t.Fatalf("unexpected ready steps: %#v", ready)
	}
}
