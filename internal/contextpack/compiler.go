// Package contextpack builds bounded, attributable model context without
// relying on a provider-specific tokenizer.
package contextpack

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/pkg/contracts"
)

const compilerVersion = "1.0.0"

type Input struct {
	DeploymentID         string
	Role, TaskID, StepID string
	BudgetLimit          int
	ReservedTokens       int
	MaxSectionsPerSource int
	Sections             []contracts.ContextSection
}

type Result struct {
	Package contracts.ContextPackage
	Omitted []contracts.ContextSection
}

// Compile selects sections deterministically. Explicit token estimates take
// precedence; otherwise a conservative four-runes-per-token estimate is used.
func Compile(input Input) (Result, error) {
	if strings.TrimSpace(input.Role) == "" || strings.TrimSpace(input.TaskID) == "" || input.BudgetLimit <= 0 || input.ReservedTokens < 0 || input.ReservedTokens >= input.BudgetLimit {
		return Result{}, contracts.NewError(contracts.ErrInvalidInput, "role, task ID and a positive usable context budget are required")
	}
	perSource := input.MaxSectionsPerSource
	if perSource <= 0 {
		perSource = 1
	}
	sections := make([]contracts.ContextSection, 0, len(input.Sections))
	for _, section := range input.Sections {
		if strings.TrimSpace(section.Type) == "" || strings.TrimSpace(section.Content) == "" || section.EstimatedTokens < 0 {
			return Result{}, contracts.NewError(contracts.ErrInvalidInput, "context sections require type, content and a non-negative token estimate")
		}
		if section.EstimatedTokens == 0 {
			section.EstimatedTokens = estimateTokens(section.Content)
		}
		section.SourceRefs = sortedUnique(section.SourceRefs)
		sections = append(sections, section)
	}
	sort.SliceStable(sections, func(i, j int) bool {
		if sections[i].Priority != sections[j].Priority {
			return sections[i].Priority > sections[j].Priority
		}
		if sections[i].EstimatedTokens != sections[j].EstimatedTokens {
			return sections[i].EstimatedTokens < sections[j].EstimatedTokens
		}
		return sectionKey(sections[i]) < sectionKey(sections[j])
	})
	available := input.BudgetLimit - input.ReservedTokens
	used := 0
	bySource := map[string]int{}
	selected := make([]contracts.ContextSection, 0, len(sections))
	omitted := make([]contracts.ContextSection, 0)
	for _, section := range sections {
		key := sourceKey(section)
		if bySource[key] >= perSource || used+section.EstimatedTokens > available {
			omitted = append(omitted, section)
			continue
		}
		selected = append(selected, section)
		used += section.EstimatedTokens
		bySource[key]++
	}
	if len(sections) > 0 && len(selected) == 0 {
		return Result{}, contracts.NewError(contracts.ErrContextOverflow, "no context section fits within the available token budget")
	}
	return Result{Package: contracts.ContextPackage{Version: contracts.SchemaVersion, ID: task.NewID("ctx"), DeploymentID: input.DeploymentID, Role: input.Role, TaskID: input.TaskID, StepID: input.StepID, Sections: selected, Budget: contracts.ContextBudget{Limit: input.BudgetLimit, Used: used, Reserved: input.ReservedTokens}, CompilerVersion: compilerVersion}, Omitted: omitted}, nil
}

func estimateTokens(content string) int {
	runes := utf8.RuneCountInString(content)
	if runes == 0 {
		return 0
	}
	return (runes + 3) / 4
}

func sourceKey(section contracts.ContextSection) string {
	if len(section.SourceRefs) == 0 {
		return "type:" + section.Type
	}
	return strings.Join(section.SourceRefs, "\x00")
}

func sectionKey(section contracts.ContextSection) string {
	return section.Type + "\x00" + sourceKey(section) + "\x00" + section.Content
}

func sortedUnique(values []string) []string {
	unique := map[string]bool{}
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			unique[trimmed] = true
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
