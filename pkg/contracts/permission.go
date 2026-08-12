package contracts

import "strings"

// PermissionMode is persisted in TaskSpec.PermissionProfileID. Keeping the
// selected mode on every execution task makes the backend, audit log and UI
// agree on the authority that was in effect for a conversation turn.
type PermissionMode string

const (
	// PermissionModePlan permits workspace inspection only. It deliberately
	// exposes no process execution and no mutation tools.
	PermissionModePlan PermissionMode = "PLAN"
	// PermissionModeEdit permits inspection and reviewable write/command
	// proposals. The user must approve the exact proposal before it runs.
	PermissionModeEdit PermissionMode = "EDIT"
	// PermissionModeDevelopment permits the bounded direct workspace tools.
	// Direct operations are still scoped to the selected workspace, time/output
	// bounded and recorded in the Agent report; they are never replayed during
	// recovery.
	PermissionModeDevelopment PermissionMode = "DEVELOPMENT"
)

func ParsePermissionMode(value string) (PermissionMode, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(PermissionModePlan):
		return PermissionModePlan, nil
	case string(PermissionModeEdit):
		return PermissionModeEdit, nil
	case string(PermissionModeDevelopment):
		return PermissionModeDevelopment, nil
	default:
		return "", NewError(ErrInvalidInput, "permission mode must be PLAN, EDIT, or DEVELOPMENT")
	}
}

func (m PermissionMode) AllowsRisk(risk RiskClass) bool {
	switch m {
	case PermissionModePlan:
		return risk == RiskRead
	case PermissionModeEdit:
		// Edit mode may propose a bounded command, but the proposal itself is a
		// reviewable write action. A plan must not be able to smuggle a direct
		// dangerous execution step into this mode.
		return risk == RiskRead || risk == RiskWrite
	case PermissionModeDevelopment:
		return risk == RiskRead || risk == RiskWrite || risk == RiskDangerous
	default:
		return false
	}
}
