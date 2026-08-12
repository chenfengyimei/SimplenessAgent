package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/internal/app"
	"github.com/xm/simplenessagent/pkg/contracts"
)

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
	desktop := &App{ctx: context.Background(), service: service}
	deployment, err := desktop.ConfigureOpenAICompatibleDeployment("local", "http://127.0.0.1:8080/v1", "model", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if deployment.CredentialRef != "desktop-session" || desktop.apiKey != "secret-value" {
		t.Fatal("desktop should retain the API key only in process memory")
	}
	items, err := desktop.ListDeployments()
	if err != nil || len(items) != 1 || items[0].CredentialRef != "desktop-session" {
		t.Fatal(items, err)
	}
}
