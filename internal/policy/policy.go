// Package policy implements deterministic capability and approval checks.
package policy

import (
	"fmt"

	"github.com/xm/simplenessagent/pkg/contracts"
)

type Profile struct{ Capabilities map[string]bool }

func ReadOnlyProfile() Profile {
	return Profile{Capabilities: map[string]bool{"fs.read": true, "process.exec.readonly": true}}
}

func (p Profile) Authorize(definition contracts.ToolDefinition) error {
	for _, capability := range definition.RequiredCapabilities {
		if !p.Capabilities[capability] {
			return contracts.NewError(contracts.ErrPermissionDenied, fmt.Sprintf("missing capability %s for tool %s", capability, definition.Name))
		}
	}
	if definition.RiskClass != contracts.RiskRead {
		return contracts.NewError(contracts.ErrApprovalRequired, "non-read tool requires a parameter-bound approval ticket")
	}
	return nil
}

func ClassifyCommand(command string) contracts.RiskClass {
	// Conservative baseline. A later parser can only refine this towards higher
	// risk; it must never make a dangerous string safer.
	if command == "" {
		return contracts.RiskRead
	}
	for _, marker := range []string{"Remove-Item", "del ", "rmdir", "format ", "git reset --hard", "Set-ExecutionPolicy"} {
		if containsFold(command, marker) {
			return contracts.RiskDangerous
		}
	}
	for _, marker := range []string{">", ">>", "Out-File", "Set-Content", "Copy-Item", "Move-Item", "git commit"} {
		if containsFold(command, marker) {
			return contracts.RiskWrite
		}
	}
	return contracts.RiskRead
}

func containsFold(value, marker string) bool {
	for index := 0; index+len(marker) <= len(value); index++ {
		match := true
		for offset := range marker {
			left, right := value[index+offset], marker[offset]
			if left >= 'A' && left <= 'Z' {
				left += 'a' - 'A'
			}
			if right >= 'A' && right <= 'Z' {
				right += 'a' - 'A'
			}
			if left != right {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
