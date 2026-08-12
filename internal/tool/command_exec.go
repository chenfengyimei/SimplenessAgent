package tool

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// RegisterDevelopmentCommandTool exposes a deliberately small, direct command
// surface. It is registered only for DEVELOPMENT tasks and never accepts a
// shell, arbitrary executable, working directory, or environment variables.
func RegisterDevelopmentCommandTool(registry *Registry, root string, beforeRun func(map[string]interface{}) (string, error)) error {
	return registerProjectCommand(registry, root, beforeRun)
}

// RegisterApprovedProjectCommand executes the exact command bound to a user
// approval ticket. Edit mode is the only caller; development mode uses the
// same bounded runner without a ticket.
func RegisterApprovedProjectCommand(registry *Registry, root string, beforeRun func(map[string]interface{}) (string, error)) error {
	if beforeRun == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "command approval callback is required")
	}
	return registerProjectCommand(registry, root, beforeRun)
}

func registerProjectCommand(registry *Registry, root string, beforeRun func(map[string]interface{}) (string, error)) error {
	if beforeRun == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "command execution callback is required")
	}
	definition := contracts.ToolDefinition{
		Version:              contracts.SchemaVersion,
		Name:                 "run_project_command",
		ToolVersion:          "1.0.0",
		Description:          "Run one bounded project command inside the selected workspace after the active permission boundary authorizes it. Command IDs and arguments are strictly allowlisted.",
		ParametersSchema:     objectSchema(map[string]interface{}{"command": stringSchema(), "arguments": arraySchema(stringSchema()), "timeout_ms": integerSchema()}, []string{"command", "arguments"}),
		RiskClass:            contracts.RiskDangerous,
		RequiredCapabilities: []string{"process.exec"},
		DefaultTimeoutMS:     30000,
		MaxOutputBytes:       commandMaxOutputBytes,
		SupportsCancel:       true,
	}
	return registry.Register(definition, func(ctx context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		proposal, err := ParseProjectCommand(args)
		if err != nil {
			return failed(started, err), nil
		}
		toolCallID, err := beforeRun(args)
		if err != nil {
			return failed(started, err), nil
		}
		result := runProjectCommand(ctx, root, started, proposal)
		result.ToolCallID = toolCallID
		return result, nil
	})
}

func runProjectCommand(ctx context.Context, root string, started time.Time, proposal CommandProposal) contracts.ToolResult {
	runContext, cancel := context.WithTimeout(ctx, time.Duration(proposal.TimeoutMS)*time.Millisecond)
	defer cancel()
	executable, arguments, supported := projectCommandInvocation(proposal)
	if !supported {
		return failed(started, contracts.NewError(contracts.ErrCapabilityUnsupported, "trusted command runtime is unavailable"))
	}
	output, exitCode, timedOut, exceeded := runFixedCommand(runContext, root, executable, arguments)
	data := commandProposalData(proposal)
	data["output"] = output
	data["exit_code"] = exitCode
	data["truncated"] = exceeded
	if timedOut {
		return failedWithData(started, contracts.NewError(contracts.ErrRequestTimeout, "command timed out"), data)
	}
	if exceeded {
		return failedWithData(started, contracts.NewError(contracts.ErrBudgetExceeded, "command output exceeded the safety limit"), data)
	}
	if exitCode != 0 {
		return failedWithData(started, contracts.NewError(contracts.ErrInvalidInput, "command failed"), data)
	}
	return success(started, projectCommandSummary(proposal), data)
}

func projectCommandInvocation(proposal CommandProposal) (string, []string, bool) {
	switch proposal.Command {
	case "go_test", "go_vet":
		goRoot := runtime.GOROOT()
		if goRoot == "" {
			return "", nil, false
		}
		verb := "test"
		if proposal.Command == "go_vet" {
			verb = "vet"
		}
		return filepath.Join(goRoot, "bin", "go"), append([]string{verb}, proposal.Arguments...), true
	case "npm_test":
		return npmExecutable(), []string{"test", "--"}, true
	case "npm_build":
		return npmExecutable(), []string{"run", "build", "--"}, true
	default:
		return "", nil, false
	}
}

func npmExecutable() string {
	if runtime.GOOS == "windows" {
		return "npm.cmd"
	}
	return "npm"
}

func projectCommandSummary(proposal CommandProposal) string {
	return strings.ReplaceAll(proposal.Command, "_", " ") + " completed"
}
