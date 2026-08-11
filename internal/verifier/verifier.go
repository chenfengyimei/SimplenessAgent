// Package verifier turns persisted evidence into deterministic acceptance
// decisions. It never trusts a model completion claim.
package verifier

import (
	"strings"
	"time"

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
	report := FinalReport{Version: contracts.SchemaVersion, TaskID: task.ID, PlanID: plan.PlanID, Passed: true, GeneratedAt: time.Now().UTC()}
	for _, criterion := range task.Spec.AcceptanceCriteria {
		check := verifyCriterion(criterion, evidence)
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
func verifyCriterion(criterion contracts.AcceptanceCriterion, evidence []contracts.Evidence) Check {
	check := Check{CriterionID: criterion.ID}
	if criterion.ID == "" {
		check.Reason = "acceptance criterion has no ID"
		return check
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
