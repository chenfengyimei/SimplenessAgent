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
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
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
	service, err := app.Open(ctx, app.Config{DataDir: *dataDir})
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
	case "task":
		return taskCommand(ctx, service, remaining[1:])
	default:
		return usage()
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
		return errors.New("task command is required: create | list | show | run | events")
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
	default:
		return errors.New("task command is required: create | list | show | run | events")
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
	return errors.New("usage: simpleness [--data-dir DIR] <init|doctor|workspace|task> ...")
}
