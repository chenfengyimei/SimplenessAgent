package app

import (
	"context"
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
