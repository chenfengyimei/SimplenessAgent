package task

import (
	"fmt"

	"github.com/xm/simplenessagent/pkg/contracts"
)

var validTransitions = map[contracts.TaskStatus]map[contracts.TaskStatus]bool{
	contracts.TaskDraft: {
		contracts.TaskPlanning: true, contracts.TaskCancelled: true,
	},
	contracts.TaskPlanning: {
		contracts.TaskReady: true, contracts.TaskWaitingUser: true, contracts.TaskFailed: true, contracts.TaskCancelled: true,
	},
	contracts.TaskReady: {
		contracts.TaskRunning: true, contracts.TaskPaused: true, contracts.TaskCancelled: true,
	},
	contracts.TaskRunning: {
		contracts.TaskVerifying: true, contracts.TaskWaitingApproval: true, contracts.TaskWaitingUser: true,
		contracts.TaskPaused: true, contracts.TaskBlocked: true, contracts.TaskFailed: true, contracts.TaskCancelled: true,
	},
	contracts.TaskVerifying: {
		contracts.TaskCompleted: true, contracts.TaskRunning: true, contracts.TaskFailed: true, contracts.TaskWaitingUser: true,
	},
	contracts.TaskWaitingApproval: {
		contracts.TaskRunning: true, contracts.TaskPaused: true, contracts.TaskCancelled: true,
	},
	contracts.TaskWaitingUser: {
		contracts.TaskPlanning: true, contracts.TaskRunning: true, contracts.TaskPaused: true, contracts.TaskCancelled: true,
	},
	contracts.TaskPaused: {
		contracts.TaskReady: true, contracts.TaskRunning: true, contracts.TaskCancelled: true,
	},
}

func CanTransition(from, to contracts.TaskStatus) bool {
	return validTransitions[from][to]
}

func ValidateTransition(from, to contracts.TaskStatus) error {
	if !CanTransition(from, to) {
		return contracts.NewError(contracts.ErrInvalidTransition, fmt.Sprintf("task cannot transition from %s to %s", from, to))
	}
	return nil
}
