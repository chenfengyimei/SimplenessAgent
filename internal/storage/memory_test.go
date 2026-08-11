package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestMemoryFTSSearchFiltersByStatusAndScope(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stamp := now()
	for _, id := range []string{"w1", "w2"} {
		if _, err = store.db.ExecContext(ctx, `INSERT INTO workspaces(id,version,name,root_path,created_at,updated_at) VALUES(?,1,?,?,?,?)`, id, id, "c:/"+id, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	first, err := store.SaveMemory(ctx, contracts.MemoryRecord{ID: "m1", Type: "DECISION", WorkspaceID: "w1", Title: "SQLite WAL", Content: "Use SQLite WAL for durable local task state.", SourceEventIDs: []string{"evt1"}, Confidence: 1, Importance: .9, Status: "ACTIVE", CreatedBy: "USER_CONFIRMED"})
	if err != nil || first.ContentHash == "" {
		t.Fatal(first, err)
	}
	if _, err = store.SaveMemory(ctx, contracts.MemoryRecord{ID: "m2", Type: "FACT", WorkspaceID: "w1", Title: "Archived SQLite", Content: "SQLite archival fact.", SourceEventIDs: []string{"evt2"}, Confidence: 1, Importance: .8, Status: "ARCHIVED", CreatedBy: "SYSTEM"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveMemory(ctx, contracts.MemoryRecord{ID: "m3", Type: "FACT", WorkspaceID: "w2", Title: "SQLite other", Content: "SQLite belongs elsewhere.", SourceEventIDs: []string{"evt3"}, Confidence: 1, Importance: .8, Status: "ACTIVE", CreatedBy: "SYSTEM"}); err != nil {
		t.Fatal(err)
	}
	results, err := store.SearchMemory(ctx, "w1", "SQLite", 10)
	if err != nil || len(results) != 1 || results[0].ID != "m1" {
		t.Fatal(results, err)
	}
}

func TestMemoryRequiresSource(t *testing.T) {
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.SaveMemory(context.Background(), contracts.MemoryRecord{ID: "m", Type: "FACT", WorkspaceID: "w", Title: "x", Content: "x", Confidence: 1, Importance: 1, Status: "ACTIVE", CreatedBy: "SYSTEM"})
	if domain, ok := err.(*contracts.Error); !ok || domain.Code != contracts.ErrInvalidInput {
		t.Fatalf("expected invalid source error, got %#v", err)
	}
}
