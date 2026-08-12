package storage

import (
	"context"
	"encoding/json"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func (s *Store) CreateConversationMessage(ctx context.Context, message contracts.ConversationMessage) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO conversation_messages(message_id,conversation_id,turn_task_id,role,content,created_at) VALUES(?,?,?,?,?,?)`, message.ID, message.ConversationID, nullIfEmpty(message.TurnTaskID), message.Role, message.Content, timestamp(message.CreatedAt))
	return err
}

func (s *Store) SetConversationID(ctx context.Context, taskID, conversationID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tasks SET conversation_id=?,updated_at=? WHERE id=?`, conversationID, now(), taskID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return contracts.NewError(contracts.ErrNotFound, "task not found")
	}
	return nil
}

func (s *Store) ListConversationMessages(ctx context.Context, conversationID string) ([]contracts.ConversationMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT message_id,conversation_id,COALESCE(turn_task_id,''),role,content,created_at FROM conversation_messages WHERE conversation_id=? ORDER BY created_at,message_id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []contracts.ConversationMessage{}
	for rows.Next() {
		var item contracts.ConversationMessage
		var created string
		if err = rows.Scan(&item.ID, &item.ConversationID, &item.TurnTaskID, &item.Role, &item.Content, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTimestamp(created)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListConversationRoots(ctx context.Context) ([]contracts.Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,version,workspace_id,COALESCE(conversation_id,''),title,goal,status,spec_json,created_at,updated_at FROM tasks WHERE conversation_id IS NULL ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []contracts.Task{}
	for rows.Next() {
		var item contracts.Task
		var spec, created, updated string
		if err = rows.Scan(&item.ID, &item.Version, &item.WorkspaceID, &item.ConversationID, &item.Title, &item.Goal, &item.Status, &spec, &created, &updated); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(spec), &item.Spec); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTimestamp(created)
		item.UpdatedAt, _ = parseTimestamp(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}
