package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func (s *Store) CreateAgentAssignment(ctx context.Context, item contracts.AgentAssignment, event contracts.EventEnvelope) error {
	tools, err := json.Marshal(item.AllowedTools)
	if err != nil {
		return err
	}
	scopes, err := json.Marshal(item.WorkspaceScopes)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_assignments(agent_id,version,task_id,step_id,deployment_id,role,depth,allowed_tools_json,workspace_scopes_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.Version, item.TaskID, item.StepID, item.DeploymentID, item.Role, item.Depth, string(tools), string(scopes), item.Status, timestamp(item.CreatedAt), timestamp(item.UpdatedAt))
	if err == nil {
		err = s.appendEventTx(ctx, tx, event)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) ListAgentAssignments(ctx context.Context, taskID string) ([]contracts.AgentAssignment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id,version,task_id,step_id,deployment_id,role,depth,allowed_tools_json,workspace_scopes_json,status,created_at,updated_at FROM agent_assignments WHERE task_id=? ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []contracts.AgentAssignment{}
	for rows.Next() {
		item, err := scanAgentAssignment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetAgentAssignment(ctx context.Context, id string) (contracts.AgentAssignment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT agent_id,version,task_id,step_id,deployment_id,role,depth,allowed_tools_json,workspace_scopes_json,status,created_at,updated_at FROM agent_assignments WHERE agent_id=?`, id)
	return scanAgentAssignment(row)
}

func (s *Store) TransitionAgentAssignment(ctx context.Context, item contracts.AgentAssignment, from, to contracts.AgentStatus, event contracts.EventEnvelope) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE agent_assignments SET status=?,updated_at=? WHERE agent_id=? AND status=?`, to, now(), item.ID, from)
	if err == nil {
		count, _ := result.RowsAffected()
		if count != 1 {
			err = contracts.NewError(contracts.ErrInvalidTransition, "agent assignment changed concurrently or transition is invalid")
		}
	}
	if err == nil {
		err = s.appendEventTx(ctx, tx, event)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type agentAssignmentRow interface{ Scan(...interface{}) error }

func scanAgentAssignment(row agentAssignmentRow) (contracts.AgentAssignment, error) {
	var item contracts.AgentAssignment
	var tools, scopes, created, updated string
	err := row.Scan(&item.ID, &item.Version, &item.TaskID, &item.StepID, &item.DeploymentID, &item.Role, &item.Depth, &tools, &scopes, &item.Status, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, contracts.NewError(contracts.ErrNotFound, "agent assignment not found")
	}
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal([]byte(tools), &item.AllowedTools)
	_ = json.Unmarshal([]byte(scopes), &item.WorkspaceScopes)
	item.CreatedAt, _ = parseTimestamp(created)
	item.UpdatedAt, _ = parseTimestamp(updated)
	return item, nil
}
