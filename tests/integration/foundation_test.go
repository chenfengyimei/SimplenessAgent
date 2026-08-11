package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestFoundationTaskLifecyclePersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	workspaceRoot := filepath.Join(t.TempDir(), "示例 项目")
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "internal", "service.go"), []byte("package internal\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dataDir := filepath.Join(t.TempDir(), "agent-data")
	service, err := app.Open(ctx, app.Config{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(ctx, "演示项目", workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	created, plan, err := service.CreateTask(ctx, app.CreateTaskInput{WorkspaceID: workspace.ID, Title: "侦察项目", Goal: "生成可验证的项目侦察报告"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != contracts.TaskReady || len(plan.Steps) != 1 {
		t.Fatalf("unexpected created task: %#v", created)
	}

	snapshot, err := service.RunTask(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Task.Status != contracts.TaskCompleted {
		t.Fatalf("task did not complete: %s", snapshot.Task.Status)
	}
	if len(snapshot.Steps) != 1 || snapshot.Steps[0].Status != contracts.StepCompleted {
		t.Fatalf("step state was not persisted: %#v", snapshot.Steps)
	}
	if len(snapshot.Steps[0].ArtifactIDs) != 1 || len(snapshot.Steps[0].EvidenceIDs) != 1 {
		t.Fatalf("reconnaissance must produce artifact and evidence: %#v", snapshot.Steps[0])
	}
	if len(snapshot.Events) < 8 {
		t.Fatalf("expected auditable lifecycle events, got %d", len(snapshot.Events))
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := app.Open(ctx, app.Config{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.GetTaskSnapshot(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Task.Status != contracts.TaskCompleted || len(recovered.Events) != len(snapshot.Events) {
		t.Fatalf("persisted snapshot changed after restart: %#v", recovered)
	}
	if matches, err := filepath.Glob(filepath.Join(dataDir, "artifacts", "*", "*")); err != nil || len(matches) == 0 {
		t.Fatalf("artifact payload was not persisted: %v %#v", err, matches)
	}
}
