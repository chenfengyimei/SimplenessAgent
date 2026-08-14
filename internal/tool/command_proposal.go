package tool

import (
	"context"
	"strings"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// CommandProposal is an exact, bounded command request. The executable is
// selected from a local allowlist; models never provide shell text.
type CommandProposal struct {
	Command   string
	Arguments []string
	TimeoutMS int
}

// RegisterCommandProposalTool exposes a review-only request for a known
// command. It does not execute anything. Edit mode can therefore request a
// project check without gaining a general process-execution capability.
func RegisterCommandProposalTool(registry *Registry, request func(CommandProposal) error) error {
	if request == nil {
		return contracts.NewError(contracts.ErrInvalidInput, "command proposal callback is required")
	}
	definition := contracts.ToolDefinition{
		Version:              contracts.SchemaVersion,
		Name:                 "propose_project_command",
		ToolVersion:          "1.0.0",
		Description:          "Propose one bounded project command for user approval. Use only the listed command IDs and explicit arguments; this does not execute a command.",
		ParametersSchema:     objectSchema(map[string]interface{}{"command": stringSchema(), "arguments": arraySchema(stringSchema()), "timeout_ms": integerSchema()}, []string{"command", "arguments"}),
		RiskClass:            contracts.RiskWrite,
		RequiredCapabilities: []string{"process.exec", "user.prompt"},
		DefaultTimeoutMS:     30000,
		MaxOutputBytes:       commandMaxOutputBytes,
		SupportsCancel:       true,
	}
	return registry.Register(definition, func(_ context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		proposal, err := ParseProjectCommand(args)
		if err != nil {
			return failed(started, err), nil
		}
		if err = request(proposal); err != nil {
			return failed(started, err), nil
		}
		return contracts.ToolResult{Version: contracts.SchemaVersion, ToolCallID: "pending-approval", Status: "WAITING_APPROVAL", Summary: "project command is waiting for user approval", Data: commandProposalData(proposal), StartedAt: started, CompletedAt: time.Now().UTC()}, nil
	})
}

// ParseProjectCommand validates the exact command request used by both
// proposal and execution paths.
func ParseProjectCommand(args map[string]interface{}) (CommandProposal, error) {
	command, ok := args["command"].(string)
	if !ok || !isProjectCommand(command) {
		return CommandProposal{}, contracts.NewError(contracts.ErrInvalidInput, "command must be one of go_test, go_vet, npm_test, or npm_build")
	}
	arguments, err := commandArguments(args["arguments"])
	if err != nil {
		return CommandProposal{}, err
	}
	if !projectCommandArgumentsAllowed(command, arguments) {
		return CommandProposal{}, contracts.NewError(contracts.ErrInvalidInput, "command arguments are not allowed for this command")
	}
	defaultTimeout := 30 * time.Second
	if command == "npm_init" || command == "npm_install" || command == "npx" || command == "pip_install" {
		defaultTimeout = 60 * time.Second
	}
	timeout := commandTimeout(args["timeout_ms"], defaultTimeout)
	return CommandProposal{Command: command, Arguments: arguments, TimeoutMS: int(timeout.Milliseconds())}, nil
}

func commandArguments(value interface{}) ([]string, error) {
	raw, ok := value.([]interface{})
	if !ok || len(raw) > 16 {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "arguments must contain at most sixteen strings")
	}
	arguments := make([]string, 0, len(raw))
	for _, item := range raw {
		argument, ok := item.(string)
		if !ok || strings.TrimSpace(argument) == "" || len(argument) > 256 || strings.ContainsAny(argument, "\r\n\x00") {
			return nil, contracts.NewError(contracts.ErrInvalidInput, "command arguments must be short non-empty strings")
		}
		arguments = append(arguments, argument)
	}
	return arguments, nil
}

func isProjectCommand(command string) bool {
	switch command {
	case "go_test", "go_vet", "npm_test", "npm_build", "npm_init", "npm_install", "npm_run", "npx", "python", "pip_install":
		return true
	default:
		return false
	}
}

func projectCommandArgumentsAllowed(command string, arguments []string) bool {
	if command == "npm_test" || command == "npm_build" {
		return len(arguments) == 0
	}
	if command == "npm_init" || command == "npm_install" {
		return len(arguments) <= 16
	}
	if command == "npm_run" {
		return len(arguments) >= 1 && len(arguments) <= 4
	}
	if command == "npx" {
		return len(arguments) >= 1 && len(arguments) <= 8
	}
	if command == "python" {
		return len(arguments) >= 1 && len(arguments) <= 8
	}
	if command == "pip_install" {
		return len(arguments) >= 1 && len(arguments) <= 16
	}
	if len(arguments) == 0 || len(arguments) > 16 {
		return false
	}
	for _, argument := range arguments {
		cleaned := strings.ReplaceAll(strings.TrimSpace(argument), "\\", "/")
		if cleaned != "." && (!strings.HasPrefix(cleaned, "./") || strings.Contains(cleaned, "..")) {
			return false
		}
	}
	return true
}

func commandProposalData(proposal CommandProposal) map[string]interface{} {
	return map[string]interface{}{"command": proposal.Command, "arguments": proposal.Arguments, "timeout_ms": proposal.TimeoutMS}
}
