package task

import (
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestTaskStateMachineRejectsInvalidTransitions(t *testing.T) {
	if err := ValidateTransition(contracts.TaskDraft, contracts.TaskCompleted); err == nil {
		t.Fatal("draft task must not complete without a plan and verification")
	}
	if err := ValidateTransition(contracts.TaskReady, contracts.TaskRunning); err != nil {
		t.Fatalf("ready task should start: %v", err)
	}
}
