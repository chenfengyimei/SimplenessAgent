package tool

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/workspace"
	"github.com/xm/simplenessagent/pkg/contracts"
)

const commandMaxOutputBytes = 128 * 1024

// RegisterReadOnlyCommands exposes fixed, non-shell verification commands.
// The model never supplies an executable name, shell syntax or environment.
func RegisterReadOnlyCommands(registry *Registry, root string) error {
	definitions := []struct {
		definition contracts.ToolDefinition
		handler    Handler
	}{
		{contracts.ToolDefinition{Version: contracts.SchemaVersion, Name: "run_go_test", ToolVersion: "1.0.0", Description: "Run go test for explicit workspace-relative packages without a shell.", ParametersSchema: objectSchema(map[string]interface{}{"packages": arraySchema(stringSchema()), "timeout_ms": integerSchema()}, []string{"packages"}), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"process.exec.readonly"}, DefaultTimeoutMS: 30000, MaxOutputBytes: commandMaxOutputBytes, SupportsCancel: true}, goCommand(root, "test")},
		{contracts.ToolDefinition{Version: contracts.SchemaVersion, Name: "run_go_vet", ToolVersion: "1.0.0", Description: "Run go vet for explicit workspace-relative packages without a shell.", ParametersSchema: objectSchema(map[string]interface{}{"packages": arraySchema(stringSchema()), "timeout_ms": integerSchema()}, []string{"packages"}), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"process.exec.readonly"}, DefaultTimeoutMS: 30000, MaxOutputBytes: commandMaxOutputBytes, SupportsCancel: true}, goCommand(root, "vet")},
		{contracts.ToolDefinition{Version: contracts.SchemaVersion, Name: "git_status", ToolVersion: "1.0.0", Description: "Read the short Git working-tree status without invoking a shell.", ParametersSchema: objectSchema(map[string]interface{}{}, nil), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: commandMaxOutputBytes}, gitStatus(root)},
		{contracts.ToolDefinition{Version: contracts.SchemaVersion, Name: "git_diff", ToolVersion: "1.0.0", Description: "Read the uncommitted Git diff for an optional workspace-relative path.", ParametersSchema: objectSchema(map[string]interface{}{"path": stringSchema()}, nil), RiskClass: contracts.RiskRead, RequiredCapabilities: []string{"fs.read"}, MaxOutputBytes: commandMaxOutputBytes}, gitDiff(root)},
	}
	for _, item := range definitions {
		if err := registry.Register(item.definition, item.handler); err != nil {
			return err
		}
	}
	return nil
}

func goCommand(root, verb string) Handler {
	return func(ctx context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		packages, err := commandPackages(args["packages"])
		if err != nil {
			return failed(started, err), nil
		}
		timeout := commandTimeout(args["timeout_ms"], 30*time.Second)
		runContext, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		goExecutable := filepath.Join(runtime.GOROOT(), "bin", "go")
		if runtime.GOROOT() == "" {
			return failed(started, contracts.NewError(contracts.ErrCapabilityUnsupported, "trusted Go runtime is unavailable")), nil
		}
		output, exitCode, timedOut, exceeded := runFixedCommand(runContext, root, goExecutable, append([]string{verb}, packages...))
		data := map[string]interface{}{"packages": packages, "output": output, "exit_code": exitCode, "truncated": exceeded}
		if timedOut {
			return failedWithData(started, contracts.NewError(contracts.ErrRequestTimeout, "command timed out"), data), nil
		}
		if exceeded {
			return failedWithData(started, contracts.NewError(contracts.ErrBudgetExceeded, "command output exceeded the safety limit"), data), nil
		}
		if exitCode != 0 {
			return failedWithData(started, contracts.NewError(contracts.ErrInvalidInput, "command failed"), data), nil
		}
		return success(started, "go "+verb+" completed", data), nil
	}
}

func gitStatus(root string) Handler {
	return func(ctx context.Context, _ map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		output, exitCode, timedOut, exceeded := runFixedCommand(ctx, root, "git", []string{"-C", root, "status", "--short", "--untracked-files=normal", "--no-renames"})
		data := map[string]interface{}{"output": output, "clean": strings.TrimSpace(output) == "", "exit_code": exitCode, "truncated": exceeded}
		if timedOut || exceeded || exitCode != 0 {
			return failedWithData(started, commandFailure(timedOut, exceeded), data), nil
		}
		return success(started, "git status read", data), nil
	}
}

func gitDiff(root string) Handler {
	return func(ctx context.Context, args map[string]interface{}) (contracts.ToolResult, error) {
		started := time.Now().UTC()
		path, _ := args["path"].(string)
		arguments := []string{"-C", root, "diff", "--no-ext-diff", "--no-color", "--unified=3", "HEAD"}
		if strings.TrimSpace(path) != "" {
			if _, err := workspace.ResolveWithin(root, path); err != nil {
				return failed(started, err), nil
			}
			arguments = append(arguments, "--", filepath.ToSlash(path))
		}
		output, exitCode, timedOut, exceeded := runFixedCommand(ctx, root, "git", arguments)
		data := map[string]interface{}{"path": path, "output": output, "exit_code": exitCode, "truncated": exceeded}
		if timedOut || exceeded || exitCode != 0 {
			return failedWithData(started, commandFailure(timedOut, exceeded), data), nil
		}
		return success(started, "git diff read", data), nil
	}
}

func runFixedCommand(ctx context.Context, directory, executable string, arguments []string) (string, int, bool, bool) {
	output := &commandOutput{limit: commandMaxOutputBytes}
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	timedOut := ctx.Err() == context.DeadlineExceeded
	exitCode := 0
	if err != nil {
		exitCode = 1
		if process, ok := err.(*exec.ExitError); ok {
			exitCode = process.ExitCode()
		}
	}
	return output.String(), exitCode, timedOut, output.exceeded
}

func commandFailure(timedOut, exceeded bool) error {
	if timedOut {
		return contracts.NewError(contracts.ErrRequestTimeout, "command timed out")
	}
	if exceeded {
		return contracts.NewError(contracts.ErrBudgetExceeded, "command output exceeded the safety limit")
	}
	return contracts.NewError(contracts.ErrInvalidInput, "command failed")
}

type commandOutput struct {
	strings.Builder
	limit    int
	exceeded bool
}

func (output *commandOutput) Write(value []byte) (int, error) {
	remaining := output.limit - output.Len()
	if remaining <= 0 {
		output.exceeded = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = output.Builder.Write(value[:remaining])
		output.exceeded = true
		return len(value), nil
	}
	return output.Builder.Write(value)
}

func commandPackages(value interface{}) ([]string, error) {
	raw, ok := value.([]interface{})
	if !ok || len(raw) == 0 || len(raw) > 16 {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "packages must contain one to sixteen explicit workspace-relative Go packages")
	}
	packages := make([]string, 0, len(raw))
	for _, item := range raw {
		name, ok := item.(string)
		name = filepath.ToSlash(strings.TrimSpace(name))
		if !ok || (name != "." && (!strings.HasPrefix(name, "./") || strings.Contains(name, "\\") || strings.Contains(name, ".."))) {
			return nil, contracts.NewError(contracts.ErrInvalidInput, "packages must be explicit workspace-relative Go packages")
		}
		packages = append(packages, name)
	}
	return packages, nil
}

func commandTimeout(value interface{}, fallback time.Duration) time.Duration {
	milliseconds, ok := integer(value)
	if !ok || milliseconds <= 0 {
		return fallback
	}
	if milliseconds > int((120 * time.Second).Milliseconds()) {
		return 120 * time.Second
	}
	return time.Duration(milliseconds) * time.Millisecond
}
