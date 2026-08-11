// Package plan validates the versioned, executable DAG. It deliberately has
// no model dependency: model output is only a candidate plan until this package
// accepts it.
package plan

import (
	"fmt"
	"sort"

	"github.com/xm/simplenessagent/pkg/contracts"
)

type ValidationResult struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

func (r ValidationResult) Valid() bool { return len(r.Errors) == 0 }

func Validate(candidate contracts.PlanVersion, maxSteps int) ValidationResult {
	result := ValidationResult{}
	if candidate.Version != contracts.SchemaVersion {
		result.Errors = append(result.Errors, "unsupported plan schema version")
	}
	if candidate.TaskID == "" || candidate.PlanID == "" {
		result.Errors = append(result.Errors, "plan_id and task_id are required")
	}
	if len(candidate.Steps) == 0 {
		result.Errors = append(result.Errors, "plan must contain at least one step")
	}
	if maxSteps > 0 && len(candidate.Steps) > maxSteps {
		result.Errors = append(result.Errors, fmt.Sprintf("plan has %d steps, exceeding budget of %d", len(candidate.Steps), maxSteps))
	}

	byID := make(map[string]contracts.StepSpec, len(candidate.Steps))
	for _, step := range candidate.Steps {
		if step.StepID == "" || step.Title == "" || step.Goal == "" {
			result.Errors = append(result.Errors, "each step requires step_id, title, and goal")
			continue
		}
		if _, exists := byID[step.StepID]; exists {
			result.Errors = append(result.Errors, "duplicate step_id: "+step.StepID)
			continue
		}
		if len(step.AllowedTools) == 0 {
			result.Errors = append(result.Errors, "step "+step.StepID+" has no allowed tools")
		}
		if len(step.AcceptanceCriteria) == 0 {
			result.Errors = append(result.Errors, "step "+step.StepID+" has no acceptance criteria")
		}
		byID[step.StepID] = step
	}
	for _, step := range candidate.Steps {
		for _, dependency := range step.Dependencies {
			if dependency == step.StepID {
				result.Errors = append(result.Errors, "step "+step.StepID+" cannot depend on itself")
			}
			if _, exists := byID[dependency]; !exists {
				result.Errors = append(result.Errors, "step "+step.StepID+" depends on unknown step "+dependency)
			}
		}
	}
	if cycle := findCycle(byID); len(cycle) > 0 {
		result.Errors = append(result.Errors, "plan contains dependency cycle: "+join(cycle, " -> "))
	}
	return result
}

func ReadySteps(plan contracts.PlanVersion, statuses map[string]contracts.StepStatus) []string {
	ready := make([]string, 0)
	for _, step := range plan.Steps {
		if statuses[step.StepID] != contracts.StepPending && statuses[step.StepID] != contracts.StepReady {
			continue
		}
		dependenciesComplete := true
		for _, dependency := range step.Dependencies {
			if statuses[dependency] != contracts.StepCompleted {
				dependenciesComplete = false
				break
			}
		}
		if dependenciesComplete {
			ready = append(ready, step.StepID)
		}
	}
	sort.Strings(ready)
	return ready
}

func findCycle(steps map[string]contracts.StepSpec) []string {
	const (
		unseen = iota
		visiting
		done
	)
	states := make(map[string]int, len(steps))
	stack := make([]string, 0, len(steps))
	var walk func(string) []string
	walk = func(id string) []string {
		states[id] = visiting
		stack = append(stack, id)
		for _, next := range steps[id].Dependencies {
			if states[next] == visiting {
				for i, candidate := range stack {
					if candidate == next {
						return append(append([]string{}, stack[i:]...), next)
					}
				}
			}
			if states[next] == unseen {
				if cycle := walk(next); len(cycle) > 0 {
					return cycle
				}
			}
		}
		stack = stack[:len(stack)-1]
		states[id] = done
		return nil
	}
	ids := make([]string, 0, len(steps))
	for id := range steps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if states[id] == unseen {
			if cycle := walk(id); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

func join(parts []string, separator string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += separator + part
	}
	return result
}
