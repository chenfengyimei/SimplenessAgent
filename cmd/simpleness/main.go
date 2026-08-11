// simpleness is the headless entry point for SimplenessAgent Core.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/internal/provider/openai"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	// `go run ./cmd/simpleness -- --data-dir ...` forwards the leading
	// separator to the program on some Go versions; accept it so the documented
	// invocation and compiled-binary invocation behave identically.
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	global := flag.NewFlagSet("simpleness", flag.ContinueOnError)
	global.SetOutput(os.Stderr)
	dataDir := global.String("data-dir", defaultDataDir(), "directory for SQLite data, artifacts, and checkpoints")
	if err := global.Parse(args); err != nil {
		return err
	}
	remaining := global.Args()
	if len(remaining) == 0 {
		return usage()
	}
	service, err := app.Open(ctx, app.Config{DataDir: *dataDir, ResolveProvider: resolveProvider})
	if err != nil {
		return err
	}
	defer service.Close()
	switch remaining[0] {
	case "init":
		return printJSON(map[string]string{"data_dir": service.DataDir(), "status": "ready"})
	case "doctor":
		return printJSON(map[string]interface{}{"status": "ready", "data_dir": service.DataDir(), "core": "sqlite-wal + artifact-store + event-log"})
	case "workspace":
		return workspaceCommand(ctx, service, remaining[1:])
	case "deployment":
		return deploymentCommand(ctx, service, remaining[1:])
	case "task":
		return taskCommand(ctx, service, remaining[1:])
	default:
		return usage()
	}
}

func resolveProvider(deployment contracts.Deployment) (contracts.ChatProvider, error) {
	if deployment.ProviderType != "openai_compatible" {
		return nil, contracts.NewError(contracts.ErrCapabilityUnsupported, "CLI supports only openai_compatible deployments")
	}
	return openai.New(openai.Config{BaseURL: deployment.Endpoint, APIKey: os.Getenv("SIMPLENESS_API_KEY"), Model: deployment.Model, DeploymentID: deployment.ID})
}

func deploymentCommand(ctx context.Context, service *app.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("deployment command is required: add | list | probe")
	}
	switch args[0] {
	case "add":
		flags := flag.NewFlagSet("deployment add", flag.ContinueOnError)
		name := flags.String("name", "", "deployment name")
		endpoint := flags.String("endpoint", "", "OpenAI-compatible base URL")
		model := flags.String("model", "", "model identifier")
		location := flags.String("location", "LOCAL", "deployment location")
		credentialRef := flags.String("credential-ref", "", "opaque credential reference; API key is read from SIMPLENESS_API_KEY")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("usage: simpleness deployment add --name NAME --endpoint URL --model MODEL [--location LOCAL] [--credential-ref REF]")
		}
		item, err := service.CreateDeployment(ctx, contracts.Deployment{Name: *name, ProviderType: "openai_compatible", Location: *location, Endpoint: *endpoint, CredentialRef: *credentialRef, Model: *model, Enabled: true})
		if err != nil {
			return err
		}
		return printJSON(item)
	case "list":
		items, err := service.ListDeployments(ctx)
		if err != nil {
			return err
		}
		return printJSON(items)
	case "probe":
		if len(args) != 2 {
			return errors.New("usage: simpleness deployment probe <deployment-id>")
		}
		health, snapshot, err := service.ProbeDeployment(ctx, args[1])
		if err != nil {
			return err
		}
		return printJSON(map[string]interface{}{"health": health, "capabilities": snapshot})
	default:
		return errors.New("deployment command is required: add | list | probe")
	}
}

func workspaceCommand(ctx context.Context, service *app.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("workspace command is required: add | list")
	}
	switch args[0] {
	case "add":
		flags := flag.NewFlagSet("workspace add", flag.ContinueOnError)
		name := flags.String("name", "", "workspace display name")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 {
			return errors.New("usage: simpleness workspace add [--name NAME] <path>")
		}
		workspace, err := service.CreateWorkspace(ctx, *name, flags.Arg(0))
		if err != nil {
			return err
		}
		return printJSON(workspace)
	case "list":
		items, err := service.ListWorkspaces(ctx)
		if err != nil {
			return err
		}
		return printJSON(items)
	default:
		return errors.New("workspace command is required: add | list")
	}
}

func taskCommand(ctx context.Context, service *app.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("task command is required: create | list | show | plan | run | run-model | events")
	}
	switch args[0] {
	case "create":
		flags := flag.NewFlagSet("task create", flag.ContinueOnError)
		workspaceID := flags.String("workspace", "", "workspace identifier")
		title := flags.String("title", "", "task title")
		goal := flags.String("goal", "", "task goal")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		item, plan, err := service.CreateTask(ctx, app.CreateTaskInput{WorkspaceID: *workspaceID, Title: *title, Goal: *goal})
		if err != nil {
			return err
		}
		return printJSON(map[string]interface{}{"task": item, "plan": plan})
	case "list":
		flags := flag.NewFlagSet("task list", flag.ContinueOnError)
		workspaceID := flags.String("workspace", "", "optional workspace identifier")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		items, err := service.ListTasks(ctx, *workspaceID)
		if err != nil {
			return err
		}
		return printJSON(items)
	case "show":
		if len(args) != 2 {
			return errors.New("usage: simpleness task show <task-id>")
		}
		snapshot, err := service.GetTaskSnapshot(ctx, args[1])
		if err != nil {
			return err
		}
		return printJSON(snapshot)
	case "events":
		if len(args) != 2 {
			return errors.New("usage: simpleness task events <task-id>")
		}
		snapshot, err := service.GetTaskSnapshot(ctx, args[1])
		if err != nil {
			return err
		}
		return printJSON(snapshot.Events)
	case "run":
		if len(args) != 2 {
			return errors.New("usage: simpleness task run <task-id>")
		}
		snapshot, err := service.RunTask(ctx, args[1])
		if err != nil {
			return err
		}
		return printJSON(snapshot)
	case "plan":
		flags := flag.NewFlagSet("task plan", flag.ContinueOnError)
		deploymentID := flags.String("deployment", "", "enabled deployment identifier")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 || strings.TrimSpace(*deploymentID) == "" {
			return errors.New("usage: simpleness task plan --deployment <deployment-id> <task-id>")
		}
		planVersion, err := service.GeneratePlan(ctx, flags.Arg(0), *deploymentID)
		if err != nil {
			return err
		}
		return printJSON(planVersion)
	case "run-model":
		flags := flag.NewFlagSet("task run-model", flag.ContinueOnError)
		deploymentID := flags.String("deployment", "", "enabled deployment identifier")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 1 || strings.TrimSpace(*deploymentID) == "" {
			return errors.New("usage: simpleness task run-model --deployment <deployment-id> <task-id>")
		}
		snapshot, err := service.RunModelStep(ctx, app.RunModelStepInput{TaskID: flags.Arg(0), DeploymentID: *deploymentID})
		if err != nil {
			return err
		}
		return printJSON(snapshot)
	default:
		return errors.New("task command is required: create | list | show | plan | run | run-model | events")
	}
}

func defaultDataDir() string {
	if value := os.Getenv("SIMPLENESS_DATA_DIR"); strings.TrimSpace(value) != "" {
		return value
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		return filepath.Join(localAppData, "SimplenessAgent")
	}
	return filepath.Join(".", "data")
}
func printJSON(value interface{}) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
func usage() error {
	return errors.New("usage: simpleness [--data-dir DIR] <init|doctor|workspace|deployment|task> ...")
}
