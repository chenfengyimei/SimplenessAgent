package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestCreateAndSearchMemory(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateMemory(ctx, contracts.MemoryRecord{Type: "CONSTRAINT", WorkspaceID: workspace.ID, Title: "No network writes", Content: "Network side effects require explicit approval.", SourceEventIDs: []string{"evt_user"}, Confidence: 1, Importance: 1, Status: "PINNED", CreatedBy: "USER_CONFIRMED"})
	if err != nil || created.ID == "" || created.ContentHash == "" {
		t.Fatal(created, err)
	}
	results, err := service.SearchMemory(ctx, workspace.ID, "Network approval", 10)
	if err != nil || len(results) != 1 || results[0].ID != created.ID {
		t.Fatal(results, err)
	}
	task, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspace.ID, Title: "context", Goal: "build memory context"})
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := service.CompileMemoryContext(ctx, MemoryContextInput{Role: "EXECUTOR", TaskID: task.ID, StepID: "step", Query: "Network approval", BudgetLimit: 100, ReservedTokens: 20, Limit: 10})
	if err != nil || len(compiled.Package.Sections) != 1 {
		t.Fatal(compiled, err)
	}
	section := compiled.Package.Sections[0]
	if section.Type != "MEMORY_CONSTRAINT" || section.SourceRefs[0] != "evt_user" && section.SourceRefs[0] != "memory:"+created.ID {
		t.Fatal(section)
	}
}
