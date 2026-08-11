// Package verifier turns persisted evidence into deterministic acceptance
// decisions. It never trusts a model completion claim.
package verifier

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

type Check struct {
	CriterionID string   `json:"criterion_id"`
	Passed      bool     `json:"passed"`
	Reason      string   `json:"reason"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
}
type FinalReport struct {
	Version     int       `json:"version"`
	TaskID      string    `json:"task_id"`
	PlanID      string    `json:"plan_id"`
	Passed      bool      `json:"passed"`
	Checks      []Check   `json:"checks"`
	GeneratedAt time.Time `json:"generated_at"`
}

func Verify(task contracts.Task, plan contracts.PlanVersion, evidence []contracts.Evidence) FinalReport {
	return VerifyInWorkspace(task, plan, evidence, "")
}
func VerifyInWorkspace(task contracts.Task, plan contracts.PlanVersion, evidence []contracts.Evidence, root string) FinalReport {
	report := FinalReport{Version: contracts.SchemaVersion, TaskID: task.ID, PlanID: plan.PlanID, Passed: true, GeneratedAt: time.Now().UTC()}
	for _, criterion := range task.Spec.AcceptanceCriteria {
		check := verifyCriterion(criterion, evidence, root)
		if !check.Passed {
			report.Passed = false
		}
		report.Checks = append(report.Checks, check)
	}
	if len(report.Checks) == 0 {
		report.Passed = false
		report.Checks = []Check{{CriterionID: "task_acceptance", Reason: "task has no acceptance criteria"}}
	}
	return report
}
func verifyCriterion(criterion contracts.AcceptanceCriterion, evidence []contracts.Evidence, root string) Check {
	check := Check{CriterionID: criterion.ID}
	if criterion.ID == "" {
		check.Reason = "acceptance criterion has no ID"
		return check
	}
	if criterion.Type == contracts.AcceptanceFileExists {
		return verifyFile(criterion, root, check)
	}
	if criterion.Type == contracts.AcceptanceDiffContains {
		return verifyDiff(criterion, root, check)
	}
	if criterion.Type != contracts.AcceptanceEvidenceExists {
		check.Reason = "acceptance type is not implemented by deterministic verifier"
		return check
	}
	kind, _ := criterion.Spec["kind"].(string)
	if strings.TrimSpace(kind) == "" {
		check.Reason = "evidence acceptance criterion requires spec.kind"
		return check
	}
	for _, item := range evidence {
		if item.Kind == kind && !item.VerifiedAt.IsZero() && item.Confidence > 0 {
			check.EvidenceIDs = append(check.EvidenceIDs, item.ID)
		}
	}
	check.Passed = len(check.EvidenceIDs) > 0
	if check.Passed {
		check.Reason = "verified matching evidence exists"
	} else {
		check.Reason = "no verified matching evidence exists"
	}
	return check
}

func verifyDiff(criterion contracts.AcceptanceCriterion, root string, check Check) Check {
	path, _ := criterion.Spec["path"].(string)
	contains, _ := criterion.Spec["contains"].(string)
	if strings.TrimSpace(root) == "" || strings.TrimSpace(path) == "" || strings.TrimSpace(contains) == "" {
		check.Reason = "diff acceptance criterion requires workspace root, spec.path and spec.contains"
		return check
	}
	target, err := workspace.ResolveWithin(root, path)
	if err != nil {
		check.Reason = "diff path is outside the authorized workspace"
		return check
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		check.Reason = "diff acceptance criterion requires a workspace file path"
		return check
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", "-C", root, "diff", "--no-ext-diff", "--no-color", "--unified=0", "HEAD", "--", relative).Output()
	if ctx.Err() != nil {
		check.Reason = "git diff verification timed out"
		return check
	}
	if err != nil {
		check.Reason = "git diff verification failed"
		return check
	}
	if !strings.Contains(string(output), contains) {
		check.Reason = "workspace diff does not contain the expected text"
		return check
	}
	check.Passed = true
	check.Reason = "workspace diff contains the expected text"
	return check
}

func verifyFile(criterion contracts.AcceptanceCriterion, root string, check Check) Check {
	path, _ := criterion.Spec["path"].(string)
	if strings.TrimSpace(root) == "" || strings.TrimSpace(path) == "" {
		check.Reason = "file acceptance criterion requires workspace root and spec.path"
		return check
	}
	target, err := workspace.ResolveWithin(root, path)
	if err != nil {
		check.Reason = "file path is outside the authorized workspace"
		return check
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		check.Reason = "required workspace file does not exist"
		return check
	}
	check.Passed = true
	check.Reason = "required workspace file exists"
	return check
}
