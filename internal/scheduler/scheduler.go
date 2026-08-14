// Package scheduler computes the Ready Set from a Plan DAG and enforces
// concurrency, budget and resource-conflict constraints. It does not execute
// steps — that remains the responsibility of the App Service.
package scheduler

import (
	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/pkg/contracts"
)

const (
	defaultMaxConcurrent = 2
	maxActiveAssignments = 4
)

// ConflictChecker determines whether two steps would conflict if run
// concurrently (e.g., writing the same file or sharing a terminal).
type ConflictChecker interface {
	Conflicts(a, b contracts.StepSpec) bool
}

// ReadySet returns the steps that can be executed right now, respecting
// dependencies, concurrency limits and resource conflicts.
type ReadySet struct {
	Steps      []contracts.StepSpec
	Skipped    []string // step IDs skipped due to conflicts
	MaxConcurrent int
}

// ComputeReadySet evaluates the DAG and returns steps whose dependencies are
// all COMPLETED and that do not conflict with any currently RUNNING step.
func ComputeReadySet(snapshot app.TaskSnapshot, maxConcurrent int, checker ConflictChecker) ReadySet {
	if maxConcurrent <= 0 {
		maxConcurrent = defaultMaxConcurrent
	}
	statusMap := make(map[string]string, len(snapshot.Steps))
	for _, step := range snapshot.Steps {
		statusMap[step.StepID] = string(step.Status)
	}
	runningCount := 0
	for _, status := range statusMap {
		if status == "RUNNING" {
			runningCount++
		}
	}
	result := ReadySet{MaxConcurrent: maxConcurrent}
	if runningCount >= maxConcurrent {
		return result
	}
	availableSlots := maxConcurrent - runningCount
	stepSpecs := make(map[string]contracts.StepSpec, len(snapshot.Plan.Steps))
	for _, step := range snapshot.Plan.Steps {
		stepSpecs[step.StepID] = step
	}
	var runningSteps []contracts.StepSpec
	for _, step := range snapshot.Plan.Steps {
		if statusMap[step.StepID] == "RUNNING" {
			runningSteps = append(runningSteps, step)
		}
	}
	for _, step := range snapshot.Plan.Steps {
		if statusMap[step.StepID] != "PENDING" && statusMap[step.StepID] != "READY" {
			continue
		}
		if !dependenciesCompleted(step, statusMap) {
			continue
		}
		conflict := false
		for _, running := range runningSteps {
			if checker != nil && checker.Conflicts(step, running) {
				conflict = true
				break
			}
		}
		if conflict {
			result.Skipped = append(result.Skipped, step.StepID)
			continue
		}
		if len(result.Steps) >= availableSlots {
			result.Skipped = append(result.Skipped, step.StepID)
			continue
		}
		result.Steps = append(result.Steps, step)
	}
	return result
}

func dependenciesCompleted(step contracts.StepSpec, statusMap map[string]string) bool {
	for _, dep := range step.Dependencies {
		if statusMap[dep] != "COMPLETED" {
			return false
		}
	}
	return true
}

// WriteConflictChecker is a basic conflict checker that prevents two WRITE
// steps from running concurrently if they share any allowed tool that writes
// files.
type WriteConflictChecker struct{}

func (WriteConflictChecker) Conflicts(a, b contracts.StepSpec) bool {
	if a.Risk != contracts.RiskWrite && b.Risk != contracts.RiskWrite {
		return false
	}
	if a.Risk == contracts.RiskDangerous || b.Risk == contracts.RiskDangerous {
		return true
	}
	for _, toolA := range a.AllowedTools {
		for _, toolB := range b.AllowedTools {
			if toolA == toolB && (toolA == "write_file" || toolA == "apply_patch" ||
				toolA == "propose_write_file" || toolA == "propose_apply_patch" ||
				toolA == "propose_file_batch") {
				return true
			}
		}
	}
	return false
}
