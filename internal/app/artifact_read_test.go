package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestReadTaskArtifactReturnsNewestArtifactOfKind(t *testing.T) {
	ctx := context.Background()
	service, err := Open(ctx, Config{DataDir: filepath.Join(t.TempDir(), "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	workspaceItem, err := service.CreateWorkspace(ctx, "artifacts", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateTask(ctx, CreateTaskInput{WorkspaceID: workspaceItem.ID, Title: "artifact order", Goal: "read latest", AcceptanceCriteria: []contracts.AcceptanceCriterion{{ID: "report", Type: contracts.AcceptanceEvidenceExists, Spec: map[string]interface{}{"kind": "AGENT_REPORT"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.persistHorizonArtifact(ctx, created.ID, "", "AGENT_REPORT", "first", map[string]interface{}{"sequence": 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.persistHorizonArtifact(ctx, created.ID, "", "AGENT_REPORT", "second", map[string]interface{}{"sequence": 2}); err != nil {
		t.Fatal(err)
	}
	content, err := service.ReadTaskArtifact(ctx, created.ID, "AGENT_REPORT")
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Sequence int `json:"sequence"`
	}
	if err = json.Unmarshal(content, &report); err != nil || report.Sequence != 2 {
		t.Fatalf("desktop report lookup did not return the newest artifact: content=%s err=%v", content, err)
	}
}
