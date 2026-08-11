package verifier

import (
	"github.com/xm/simplenessagent/pkg/contracts"
	"testing"
	"time"
)

func TestVerifyRequiresVerifiedMatchingEvidence(t *testing.T) {
	task := contracts.Task{ID: "t", Spec: contracts.TaskSpec{AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "a", Type: contracts.AcceptanceEvidenceExists, Spec: map[string]interface{}{"kind": "REPORT"}}}}}
	report := Verify(task, contracts.PlanVersion{PlanID: "p"}, []contracts.Evidence{{ID: "e", Kind: "REPORT", VerifiedAt: time.Now(), Confidence: 1}})
	if !report.Passed || len(report.Checks[0].EvidenceIDs) != 1 {
		t.Fatal(report)
	}
	report = Verify(task, contracts.PlanVersion{}, []contracts.Evidence{{Kind: "REPORT", Confidence: 1}})
	if report.Passed {
		t.Fatal(report)
	}
}
