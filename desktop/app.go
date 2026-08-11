package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/pkg/contracts"
)

// App struct
type App struct {
	ctx     context.Context
	service *app.Service
	openErr error
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	dataDir, err := desktopDataDir()
	if err != nil {
		a.openErr = err
		return
	}
	a.service, a.openErr = app.Open(ctx, app.Config{DataDir: dataDir})
}

func (a *App) shutdown(_ context.Context) {
	if a.service != nil {
		_ = a.service.Close()
	}
}

func (a *App) core() (*app.Service, error) {
	if a.openErr != nil {
		return nil, a.openErr
	}
	return a.service, nil
}

// ListTasks is a read-only query over Core snapshots. Desktop commands below
// call the same App Service validation and state-machine boundaries as the CLI.
func (a *App) ListTasks() ([]app.TaskSnapshot, error) {
	service, err := a.core()
	if err != nil {
		return nil, err
	}
	tasks, err := service.ListTasks(a.ctx, "")
	if err != nil {
		return nil, err
	}
	result := make([]app.TaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		snapshot, err := service.GetTaskSnapshot(a.ctx, task.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (a *App) ListWorkspaces() ([]contracts.Workspace, error) {
	service, err := a.core()
	if err != nil {
		return nil, err
	}
	return service.ListWorkspaces(a.ctx)
}

func (a *App) CreateWorkspace(name, path string) (contracts.Workspace, error) {
	service, err := a.core()
	if err != nil {
		return contracts.Workspace{}, err
	}
	return service.CreateWorkspace(a.ctx, name, path)
}

func (a *App) CreateTask(workspaceID, title, goal string) (app.TaskSnapshot, error) {
	service, err := a.core()
	if err != nil {
		return app.TaskSnapshot{}, err
	}
	created, _, err := service.CreateTask(a.ctx, app.CreateTaskInput{WorkspaceID: workspaceID, Title: title, Goal: goal})
	if err != nil {
		return app.TaskSnapshot{}, err
	}
	return service.GetTaskSnapshot(a.ctx, created.ID)
}

func (a *App) DataDir() (string, error) {
	service, err := a.core()
	if err != nil {
		return "", err
	}
	return service.DataDir(), nil
}

func desktopDataDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "SimplenessAgent"), nil
}
