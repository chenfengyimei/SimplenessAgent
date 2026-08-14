package scheduler

import (
	"testing"

	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestComputeReadySet_NoDependencies(t *testing.T) {
	snapshot := app.TaskSnapshot{
		Task: contracts.Task{ID: "tsk_1"},
		Plan: contracts.PlanVersion{Steps: []contracts.StepSpec{
			{StepID: "s1", Risk: contracts.RiskRead},
			{StepID: "s2", Risk: contracts.RiskRead},
		}},
		Steps: []contracts.StepRuntime{
			{StepID: "s1", Status: "PENDING"},
			{StepID: "s2", Status: "PENDING"},
		},
	}
	result := ComputeReadySet(snapshot, 2, WriteConflictChecker{})
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 ready steps, got %d", len(result.Steps))
	}
}

func TestComputeReadySet_DependencyBlocks(t *testing.T) {
	snapshot := app.TaskSnapshot{
		Task: contracts.Task{ID: "tsk_1"},
		Plan: contracts.PlanVersion{Steps: []contracts.StepSpec{
			{StepID: "s1", Risk: contracts.RiskRead, Dependencies: []string{"s2"}},
			{StepID: "s2", Risk: contracts.RiskRead},
		}},
		Steps: []contracts.StepRuntime{
			{StepID: "s1", Status: "PENDING"},
			{StepID: "s2", Status: "PENDING"},
		},
	}
	result := ComputeReadySet(snapshot, 2, WriteConflictChecker{})
	if len(result.Steps) != 1 || result.Steps[0].StepID != "s2" {
		t.Errorf("expected only s2 ready, got %v", result.Steps)
	}
}

func TestComputeReadySet_ConcurrencyLimit(t *testing.T) {
	snapshot := app.TaskSnapshot{
		Task: contracts.Task{ID: "tsk_1"},
		Plan: contracts.PlanVersion{Steps: []contracts.StepSpec{
			{StepID: "s1", Risk: contracts.RiskRead},
			{StepID: "s2", Risk: contracts.RiskRead},
			{StepID: "s3", Risk: contracts.RiskRead},
		}},
		Steps: []contracts.StepRuntime{
			{StepID: "s1", Status: "RUNNING"},
			{StepID: "s2", Status: "PENDING"},
			{StepID: "s3", Status: "PENDING"},
		},
	}
	result := ComputeReadySet(snapshot, 2, WriteConflictChecker{})
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 ready step (1 slot used by running), got %d", len(result.Steps))
	}
}

func TestComputeReadySet_WriteConflictBlocks(t *testing.T) {
	snapshot := app.TaskSnapshot{
		Task: contracts.Task{ID: "tsk_1"},
		Plan: contracts.PlanVersion{Steps: []contracts.StepSpec{
			{StepID: "s1", Risk: contracts.RiskWrite, AllowedTools: []string{"write_file"}},
			{StepID: "s2", Risk: contracts.RiskWrite, AllowedTools: []string{"write_file"}},
		}},
		Steps: []contracts.StepRuntime{
			{StepID: "s1", Status: "RUNNING"},
			{StepID: "s2", Status: "PENDING"},
		},
	}
	result := ComputeReadySet(snapshot, 2, WriteConflictChecker{})
	if len(result.Steps) != 0 {
		t.Errorf("expected 0 ready steps (write conflict), got %d", len(result.Steps))
	}
	if len(result.Skipped) != 1 {
		t.Errorf("expected 1 skipped step, got %d", len(result.Skipped))
	}
}
