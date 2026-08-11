package verifier

import (
	"github.com/xm/simplenessagent/pkg/contracts"
	"os"
	"os/exec"
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

func TestVerifyDiffContainsWithinWorkspace(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	target := filepath.Join(root, "report.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "report.txt")
	runGit(t, root, "commit", "-m", "initial")
	if err := os.WriteFile(target, []byte("before\nafter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := contracts.Task{ID: "t", Spec: contracts.TaskSpec{AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "diff", Type: contracts.AcceptanceDiffContains, Spec: map[string]interface{}{"path": "report.txt", "contains": "+after"}}}}}
	if !VerifyInWorkspace(task, contracts.PlanVersion{}, nil, root).Passed {
		t.Fatal("diff criterion should pass")
	}
	task.Spec.AcceptanceCriteria[0].Spec["path"] = "../outside"
	if VerifyInWorkspace(task, contracts.PlanVersion{}, nil, root).Passed {
		t.Fatal("diff path escape must fail")
	}
}

func runGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
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
