package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func TestIntentIdempotencyAndApprovalConsumption(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stamp := now()
	_, err = store.db.ExecContext(ctx, `INSERT INTO workspaces(id,version,name,root_path,created_at,updated_at) VALUES('w',1,'w','c:/w',?,?)`, stamp, stamp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO tasks(id,version,workspace_id,title,goal,status,spec_json,created_at,updated_at) VALUES('t',1,'w','t','g','READY','{}',?,?)`, stamp, stamp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO plan_versions(plan_id,version,task_id,revision,reason,summary,plan_json,created_at) VALUES('p',1,'t',1,'r','s','{}',?)`, stamp)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO steps(step_id,plan_id,task_id,spec_json,status,created_at,updated_at) VALUES('s','p','t','{}','PENDING',?,?)`, stamp, stamp)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.RecordToolIntent(ctx, contracts.ToolCallRecord{ID: "i1", StepID: "s", ToolName: "write", ArgumentsHash: "sha256:x", IdempotencyKey: "intent:x", Risk: contracts.RiskWrite})
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.RecordToolIntent(ctx, contracts.ToolCallRecord{ID: "i2", StepID: "s", ToolName: "write", ArgumentsHash: "sha256:x", IdempotencyKey: "intent:x", Risk: contracts.RiskWrite})
	if err != nil || again.ID != first.ID {
		t.Fatalf("intent was not reused %#v %v", again, err)
	}
	if err = store.CreateApproval(ctx, contracts.ApprovalTicket{ID: "a", TaskID: "t", StepID: "s", ToolName: "write", ArgumentsHash: "sha256:x", Decision: "APPROVED", UsesRemaining: 1, ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err = store.ConsumeApproval(ctx, "t", "s", "write", "sha256:x"); err != nil {
		t.Fatal(err)
	}
	if err = store.ConsumeApproval(ctx, "t", "s", "write", "sha256:x"); err == nil {
		t.Fatal("ticket reuse must fail")
	}
}
