package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/pkg/contracts"
)

type memoryCredentialStore struct{ values map[string]string }

func (s *memoryCredentialStore) Save(id, key string) error      { s.values[id] = key; return nil }
func (s *memoryCredentialStore) Load(id string) (string, error) { return s.values[id], nil }
func (s *memoryCredentialStore) Delete(id string) error         { delete(s.values, id); return nil }

func TestDesktopTaskUsesAgentReportAcceptance(t *testing.T) {
	service, err := app.Open(context.Background(), app.Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	desktop := &App{ctx: context.Background(), service: service}
	workspace, err := desktop.CreateWorkspace("demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := desktop.CreateTask(workspace.ID, "inspect", "inspect workspace")
	if err != nil {
		t.Fatal(err)
	}
	criteria := snapshot.Task.Spec.AcceptanceCriteria
	if len(criteria) != 1 || criteria[0].Type != contracts.AcceptanceEvidenceExists || criteria[0].Spec["kind"] != "AGENT_REPORT" {
		t.Fatalf("unexpected desktop task acceptance: %#v", criteria)
	}
}

func TestDesktopDeploymentNeverPersistsAPIKey(t *testing.T) {
	service, err := app.Open(context.Background(), app.Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	credentials := &memoryCredentialStore{values: map[string]string{}}
	desktop := &App{ctx: context.Background(), service: service, apiKeys: map[string]string{}, credentials: credentials}
	deployment, err := desktop.ConfigureOpenAICompatibleDeployment("local", "http://127.0.0.1:8080/v1", "model", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if deployment.CredentialRef != "windows-credential-manager" || desktop.apiKeys[deployment.ID] != "secret-value" {
		t.Fatal("desktop should retain the API key in the Windows credential manager")
	}
	items, err := desktop.ListDeployments()
	if err != nil || len(items) != 1 || items[0].CredentialRef != "windows-credential-manager" {
		t.Fatal(items, err)
	}
	if key, err := credentials.Load(deployment.ID); err != nil || key != "secret-value" {
		t.Fatalf("credential manager did not retain the API key: %q, %v", key, err)
	}
}
