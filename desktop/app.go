package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/xm/simplenessagent/internal/app"
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

// ListTasks is deliberately a read-only desktop binding. Commands, provider
// credentials and tool execution remain behind the Core CLI/App Service API.
func (a *App) ListTasks() ([]app.TaskSnapshot, error) {
	if a.openErr != nil {
		return nil, a.openErr
	}
	tasks, err := a.service.ListTasks(a.ctx, "")
	if err != nil {
		return nil, err
	}
	result := make([]app.TaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		snapshot, err := a.service.GetTaskSnapshot(a.ctx, task.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	return result, nil
}

func (a *App) DataDir() (string, error) {
	if a.openErr != nil {
		return "", a.openErr
	}
	return a.service.DataDir(), nil
}

func desktopDataDir() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "SimplenessAgent"), nil
}
