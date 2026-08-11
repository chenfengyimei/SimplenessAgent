package verifier

import (
	"github.com/xm/simplenessagent/pkg/contracts"
	"os"
	"path/filepath"
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

func TestVerifyFileExistsWithinWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "report.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := contracts.Task{ID: "t", Spec: contracts.TaskSpec{AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "file", Type: contracts.AcceptanceFileExists, Spec: map[string]interface{}{"path": "report.txt"}}}}}
	if !VerifyInWorkspace(task, contracts.PlanVersion{}, nil, root).Passed {
		t.Fatal("file criterion should pass")
	}
	task.Spec.AcceptanceCriteria[0].Spec["path"] = "../outside"
	if VerifyInWorkspace(task, contracts.PlanVersion{}, nil, root).Passed {
		t.Fatal("path escape must fail")
	}
}
