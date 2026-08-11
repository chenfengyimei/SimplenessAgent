package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func (s *Store) SaveMemory(ctx context.Context, item contracts.MemoryRecord) (contracts.MemoryRecord, error) {
	if err := normalizeMemory(&item); err != nil {
		return contracts.MemoryRecord{}, err
	}
	tags, _ := json.Marshal(item.Tags)
	events, _ := json.Marshal(item.SourceEventIDs)
	artifacts, _ := json.Marshal(item.SourceArtifactIDs)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return contracts.MemoryRecord{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO memory_records(memory_id,version,type,workspace_id,task_id,title,content,tags_json,source_event_ids_json,source_artifact_ids_json,confidence,importance,status,valid_from,valid_until,supersedes_memory_id,created_by,content_hash,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.Version, item.Type, item.WorkspaceID, nullIfEmpty(item.TaskID), item.Title, item.Content, string(tags), string(events), string(artifacts), item.Confidence, item.Importance, item.Status, timestamp(item.ValidFrom), nullableTimestamp(item.ValidUntil), nullIfEmpty(item.Supersedes), item.CreatedBy, item.ContentHash, timestamp(item.CreatedAt))
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO memory_fts(memory_id,title,content,tags) VALUES(?,?,?,?)`, item.ID, item.Title, item.Content, strings.Join(item.Tags, " "))
	}
	if err != nil {
		_ = tx.Rollback()
		return contracts.MemoryRecord{}, err
	}
	if err = tx.Commit(); err != nil {
		return contracts.MemoryRecord{}, err
	}
	return item, nil
}

func (s *Store) SearchMemory(ctx context.Context, workspaceID, query string, limit int) ([]contracts.MemoryRecord, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(query) == "" {
		return nil, contracts.NewError(contracts.ErrInvalidInput, "workspace ID and memory query are required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	match := ftsMatch(query)
	rows, err := s.db.QueryContext(ctx, `SELECT m.memory_id,m.version,m.type,m.workspace_id,COALESCE(m.task_id,''),m.title,m.content,m.tags_json,m.source_event_ids_json,m.source_artifact_ids_json,m.confidence,m.importance,m.status,m.valid_from,COALESCE(m.valid_until,''),COALESCE(m.supersedes_memory_id,''),m.created_by,m.content_hash,m.created_at FROM memory_fts JOIN memory_records m ON m.memory_id=memory_fts.memory_id WHERE memory_fts MATCH ? AND m.workspace_id=? AND m.status IN ('ACTIVE','PINNED') AND (m.valid_until IS NULL OR m.valid_until>?) ORDER BY bm25(memory_fts),m.importance DESC,m.created_at DESC LIMIT ?`, match, workspaceID, now(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []contracts.MemoryRecord{}
	for rows.Next() {
		item, scanErr := scanMemory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type memoryRow interface{ Scan(...interface{}) error }

func scanMemory(row memoryRow) (contracts.MemoryRecord, error) {
	var item contracts.MemoryRecord
	var tags, events, artifacts, validFrom, validUntil, created string
	err := row.Scan(&item.ID, &item.Version, &item.Type, &item.WorkspaceID, &item.TaskID, &item.Title, &item.Content, &tags, &events, &artifacts, &item.Confidence, &item.Importance, &item.Status, &validFrom, &validUntil, &item.Supersedes, &item.CreatedBy, &item.ContentHash, &created)
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal([]byte(tags), &item.Tags)
	_ = json.Unmarshal([]byte(events), &item.SourceEventIDs)
	_ = json.Unmarshal([]byte(artifacts), &item.SourceArtifactIDs)
	item.ValidFrom, _ = parseTimestamp(validFrom)
	if validUntil != "" {
		item.ValidUntil, _ = parseTimestamp(validUntil)
	}
	item.CreatedAt, _ = parseTimestamp(created)
	return item, nil
}

func normalizeMemory(item *contracts.MemoryRecord) error {
	if item.ID == "" || strings.TrimSpace(item.Type) == "" || strings.TrimSpace(item.WorkspaceID) == "" || strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.Content) == "" || strings.TrimSpace(item.Status) == "" || strings.TrimSpace(item.CreatedBy) == "" {
		return contracts.NewError(contracts.ErrInvalidInput, "complete scoped memory record is required")
	}
	if len(item.SourceEventIDs) == 0 && len(item.SourceArtifactIDs) == 0 {
		return contracts.NewError(contracts.ErrInvalidInput, "memory record requires at least one event or artifact source")
	}
	if item.Confidence < 0 || item.Confidence > 1 || item.Importance < 0 || item.Importance > 1 {
		return contracts.NewError(contracts.ErrInvalidInput, "memory confidence and importance must be between zero and one")
	}
	if item.Version == 0 {
		item.Version = contracts.SchemaVersion
	}
	if item.Version != contracts.SchemaVersion {
		return contracts.NewError(contracts.ErrInvalidInput, "unsupported memory schema version")
	}
	if item.ValidFrom.IsZero() {
		item.ValidFrom = time.Now().UTC()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if !item.ValidUntil.IsZero() && !item.ValidUntil.After(item.ValidFrom) {
		return contracts.NewError(contracts.ErrInvalidInput, "memory valid_until must be after valid_from")
	}
	item.Tags = normalizedStrings(item.Tags)
	item.SourceEventIDs = normalizedStrings(item.SourceEventIDs)
	item.SourceArtifactIDs = normalizedStrings(item.SourceArtifactIDs)
	digest := sha256.Sum256([]byte(item.Type + "\x00" + item.Title + "\x00" + item.Content))
	item.ContentHash = "sha256:" + hex.EncodeToString(digest[:])
	return nil
}

func normalizedStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func ftsMatch(query string) string {
	parts := strings.Fields(query)
	for index, part := range parts {
		parts[index] = "\"" + strings.ReplaceAll(part, "\"", "\"\"") + "\""
	}
	return strings.Join(parts, " AND ")
}

func nullableTimestamp(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return timestamp(value)
}
