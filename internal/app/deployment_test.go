package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/internal/provider/mock"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestDeploymentProbePersistsCapabilitySnapshot(t *testing.T) {
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(deployment contracts.Deployment) (contracts.ChatProvider, error) { return mock.Provider{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "local", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1:8080/v1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	health, snapshot, err := service.ProbeDeployment(context.Background(), deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !health.Healthy || snapshot.ID == "" || snapshot.DeploymentID != deployment.ID {
		t.Fatalf("unexpected probe: %#v %#v", health, snapshot)
	}
	deployments, err := service.ListDeployments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(deployments) != 1 || deployments[0].CapabilitySnapshotID != snapshot.ID {
		t.Fatalf("snapshot linkage was not persisted: %#v", deployments)
	}
}

func TestGeneratePlanPersistsModelRevision(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	response := `{"summary":"model plan","reason":"MODEL","steps":[{"version":1,"step_id":"inspect","title":"Inspect","goal":"Read files","allowed_tools":["list_files"],"workspace_scopes":["."],"acceptance_criteria":[{"id":"evidence","type":"EVIDENCE_EXISTS","description":"report","spec":{}}],"risk":"READ","budget":{"max_attempts":1,"max_iterations":2,"max_duration_ms":1000,"max_input_tokens":10,"max_output_tokens":10}}]}`
	service, err := Open(context.Background(), Config{DataDir: filepath.Join(t.TempDir(), "data"), ResolveProvider: func(contracts.Deployment) (contracts.ChatProvider, error) {
		return mock.Provider{Response: response}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspace, err := service.CreateWorkspace(context.Background(), "demo", root)
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(context.Background(), CreateTaskInput{WorkspaceID: workspace.ID, Title: "demo", Goal: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := service.CreateDeployment(context.Background(), contracts.Deployment{Name: "model", ProviderType: "openai_compatible", Location: "LOCAL", Endpoint: "http://127.0.0.1", Model: "test", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := service.GeneratePlan(context.Background(), created.ID, deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if generated.Revision != 2 || generated.ParentPlanID == "" {
		t.Fatalf("unexpected plan revision: %#v", generated)
	}
	snapshot, err := service.GetTaskSnapshot(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Plan.PlanID != generated.PlanID || len(snapshot.Events) < 5 {
		t.Fatalf("plan was not persisted with event: %#v", snapshot)
	}
}
