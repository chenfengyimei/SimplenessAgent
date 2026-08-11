package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

const toolIntentRecorded = "INTENT_RECORDED"

func (s *Store) RecordToolIntent(ctx context.Context, record contracts.ToolCallRecord) (contracts.ToolCallRecord, error) {
	if err := normalizeIntent(&record); err != nil {
		return contracts.ToolCallRecord{}, err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO tool_calls(tool_call_id,version,run_id,step_id,tool_name,normalized_arguments_hash,idempotency_key,risk_class,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, record.ID, record.Version, nullIfEmpty(record.RunID), record.StepID, record.ToolName, record.ArgumentsHash, record.IdempotencyKey, record.Risk, record.Status, timestamp(record.CreatedAt), timestamp(record.UpdatedAt))
	if err == nil {
		return record, nil
	}
	existing, lookupErr := s.GetToolIntent(ctx, record.IdempotencyKey)
	if lookupErr == nil {
		return existing, nil
	}
	return contracts.ToolCallRecord{}, err
}

// RecordToolIntentWithEvent commits a newly recorded intent and its task audit
// event together. A duplicate idempotency key returns the existing intent
// without creating a duplicate event.
func (s *Store) RecordToolIntentWithEvent(ctx context.Context, record contracts.ToolCallRecord, event contracts.EventEnvelope) (contracts.ToolCallRecord, error) {
	if err := normalizeIntent(&record); err != nil {
		return contracts.ToolCallRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contracts.ToolCallRecord{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tool_calls(tool_call_id,version,run_id,step_id,tool_name,normalized_arguments_hash,idempotency_key,risk_class,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, record.ID, record.Version, nullIfEmpty(record.RunID), record.StepID, record.ToolName, record.ArgumentsHash, record.IdempotencyKey, record.Risk, record.Status, timestamp(record.CreatedAt), timestamp(record.UpdatedAt))
	if err == nil {
		err = s.appendEventTx(ctx, tx, event)
	}
	if err != nil {
		_ = tx.Rollback()
		existing, lookupErr := s.GetToolIntent(ctx, record.IdempotencyKey)
		if lookupErr == nil {
			return existing, nil
		}
		return contracts.ToolCallRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return contracts.ToolCallRecord{}, err
	}
	return record, nil
}

func normalizeIntent(record *contracts.ToolCallRecord) error {
	if record.ID == "" || record.StepID == "" || record.ToolName == "" || record.ArgumentsHash == "" || record.IdempotencyKey == "" {
		return contracts.NewError(contracts.ErrInvalidInput, "complete tool intent is required")
	}
	if record.Version == 0 {
		record.Version = contracts.SchemaVersion
	}
	if record.Status == "" {
		record.Status = toolIntentRecorded
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = parseNow()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	return nil
}
func (s *Store) GetToolIntent(ctx context.Context, key string) (contracts.ToolCallRecord, error) {
	var r contracts.ToolCallRecord
	var run sql.NullString
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT tool_call_id,version,run_id,step_id,tool_name,normalized_arguments_hash,idempotency_key,risk_class,status,created_at,updated_at FROM tool_calls WHERE idempotency_key=?`, key).Scan(&r.ID, &r.Version, &run, &r.StepID, &r.ToolName, &r.ArgumentsHash, &r.IdempotencyKey, &r.Risk, &r.Status, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return r, contracts.NewError(contracts.ErrNotFound, "tool intent not found")
	}
	if err != nil {
		return r, err
	}
	r.RunID = run.String
	r.CreatedAt, _ = parseTimestamp(created)
	r.UpdatedAt, _ = parseTimestamp(updated)
	return r, nil
}

func (s *Store) UpdateToolIntentStatus(ctx context.Context, id, status string) error {
	if id == "" || status == "" {
		return contracts.NewError(contracts.ErrInvalidInput, "tool intent id and status are required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE tool_calls SET status=?,updated_at=? WHERE tool_call_id=?`, status, now(), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return contracts.NewError(contracts.ErrNotFound, "tool intent not found")
	}
	return nil
}
func (s *Store) CreateApproval(ctx context.Context, ticket contracts.ApprovalTicket) error {
	if err := validateApproval(&ticket); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO approvals(approval_id,task_id,step_id,tool_name,normalized_arguments_hash,decision,expires_at,uses_remaining,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, ticket.ID, ticket.TaskID, nullIfEmpty(ticket.StepID), ticket.ToolName, ticket.ArgumentsHash, ticket.Decision, expiry(ticket.ExpiresAt), ticket.UsesRemaining, timestamp(ticket.CreatedAt))
	return err
}

// CreateApprovalWithEvent keeps a user approval and its task audit record in
// one transaction.
func (s *Store) CreateApprovalWithEvent(ctx context.Context, ticket contracts.ApprovalTicket, event contracts.EventEnvelope) error {
	if err := validateApproval(&ticket); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO approvals(approval_id,task_id,step_id,tool_name,normalized_arguments_hash,decision,expires_at,uses_remaining,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, ticket.ID, ticket.TaskID, nullIfEmpty(ticket.StepID), ticket.ToolName, ticket.ArgumentsHash, ticket.Decision, expiry(ticket.ExpiresAt), ticket.UsesRemaining, timestamp(ticket.CreatedAt))
	if err == nil {
		err = s.appendEventTx(ctx, tx, event)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func validateApproval(ticket *contracts.ApprovalTicket) error {
	if ticket.ID == "" || ticket.TaskID == "" || ticket.ToolName == "" || ticket.ArgumentsHash == "" || ticket.Decision == "" || ticket.UsesRemaining <= 0 {
		return contracts.NewError(contracts.ErrInvalidInput, "complete approval ticket is required")
	}
	if ticket.CreatedAt.IsZero() {
		ticket.CreatedAt = parseNow()
	}
	return nil
}

func expiry(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return timestamp(value)
}
func (s *Store) ConsumeApproval(ctx context.Context, taskID, stepID, toolName, argumentsHash string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE approvals SET uses_remaining=uses_remaining-1 WHERE approval_id=(SELECT approval_id FROM approvals WHERE task_id=? AND (step_id=? OR step_id IS NULL) AND tool_name=? AND normalized_arguments_hash=? AND decision='APPROVED' AND uses_remaining>0 AND (expires_at IS NULL OR expires_at>?) ORDER BY created_at LIMIT 1)`, taskID, stepID, toolName, argumentsHash, now())
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return contracts.NewError(contracts.ErrApprovalRequired, "no valid approval ticket matches this tool intent")
	}
	return nil
}
func parseNow() time.Time { return time.Now().UTC() }
