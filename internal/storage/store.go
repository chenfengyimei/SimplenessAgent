// Package storage owns the SQLite transaction boundary. Business state changes
// and their EventEnvelope are committed in one transaction.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xm/simplenessagent/migrations"
	"github.com/xm/simplenessagent/pkg/contracts"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;"); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.Migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	migrationsToApply, err := migrations.All()
	if err != nil {
		return err
	}
	for _, migration := range migrationsToApply {
		var exists int
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", migration.Version).Scan(&exists)
		if err != nil && !isMissingTable(err) {
			return err
		}
		if exists > 0 {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, migration.SQL); err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)", migration.Version, migration.Name, now())
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", migration.Name, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func isMissingTable(err error) bool {
	return err != nil && (contains(err.Error(), "no such table") || contains(err.Error(), "does not exist"))
}
func contains(value, substring string) bool {
	return len(value) >= len(substring) && (value == substring || index(value, substring) >= 0)
}
func index(value, substring string) int {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return i
		}
	}
	return -1
}
func now() string                                    { return time.Now().UTC().Format(time.RFC3339Nano) }
func timestamp(value time.Time) string               { return value.UTC().Format(time.RFC3339Nano) }
func parseTimestamp(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func (s *Store) CreateWorkspace(ctx context.Context, workspace contracts.Workspace) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO workspaces(id,version,name,root_path,created_at,updated_at) VALUES(?,?,?,?,?,?)`, workspace.ID, workspace.Version, workspace.Name, workspace.RootPath, timestamp(workspace.CreatedAt), timestamp(workspace.UpdatedAt))
	return err
}

func (s *Store) ListWorkspaces(ctx context.Context) ([]contracts.Workspace, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,version,name,root_path,created_at,updated_at FROM workspaces ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []contracts.Workspace{}
	for rows.Next() {
		var item contracts.Workspace
		var created, updated string
		if err := rows.Scan(&item.ID, &item.Version, &item.Name, &item.RootPath, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTimestamp(created)
		item.UpdatedAt, _ = parseTimestamp(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetWorkspace(ctx context.Context, id string) (contracts.Workspace, error) {
	var item contracts.Workspace
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,version,name,root_path,created_at,updated_at FROM workspaces WHERE id=?`, id).Scan(&item.ID, &item.Version, &item.Name, &item.RootPath, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, contracts.NewError(contracts.ErrNotFound, "workspace not found")
	}
	if err != nil {
		return item, err
	}
	item.CreatedAt, _ = parseTimestamp(created)
	item.UpdatedAt, _ = parseTimestamp(updated)
	return item, nil
}

func (s *Store) CreateTask(ctx context.Context, task contracts.Task, event contracts.EventEnvelope) error {
	specJSON, err := json.Marshal(task.Spec)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tasks(id,version,workspace_id,title,goal,status,spec_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, task.ID, task.Version, task.WorkspaceID, task.Title, task.Goal, task.Status, string(specJSON), timestamp(task.CreatedAt), timestamp(task.UpdatedAt))
	if err == nil {
		err = s.appendEventTx(ctx, tx, event)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) GetTask(ctx context.Context, id string) (contracts.Task, error) {
	var item contracts.Task
	var spec, created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,version,workspace_id,title,goal,status,spec_json,created_at,updated_at FROM tasks WHERE id=?`, id).Scan(&item.ID, &item.Version, &item.WorkspaceID, &item.Title, &item.Goal, &item.Status, &spec, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, contracts.NewError(contracts.ErrNotFound, "task not found")
	}
	if err != nil {
		return item, err
	}
	if err = json.Unmarshal([]byte(spec), &item.Spec); err != nil {
		return item, err
	}
	item.CreatedAt, _ = parseTimestamp(created)
	item.UpdatedAt, _ = parseTimestamp(updated)
	return item, nil
}

func (s *Store) ListTasks(ctx context.Context, workspaceID string) ([]contracts.Task, error) {
	query := `SELECT id,version,workspace_id,title,goal,status,spec_json,created_at,updated_at FROM tasks`
	args := []interface{}{}
	if workspaceID != "" {
		query += ` WHERE workspace_id=?`
		args = append(args, workspaceID)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []contracts.Task{}
	for rows.Next() {
		var item contracts.Task
		var spec, created, updated string
		if err := rows.Scan(&item.ID, &item.Version, &item.WorkspaceID, &item.Title, &item.Goal, &item.Status, &spec, &created, &updated); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(spec), &item.Spec); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = parseTimestamp(created)
		item.UpdatedAt, _ = parseTimestamp(updated)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) TransitionTask(ctx context.Context, taskID string, from, to contracts.TaskStatus, event contracts.EventEnvelope) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status=?,updated_at=? WHERE id=? AND status=?`, to, now(), taskID, from)
	if err == nil {
		var affected int64
		affected, _ = result.RowsAffected()
		if affected != 1 {
			err = contracts.NewError(contracts.ErrInvalidTransition, "task status changed concurrently or transition is invalid")
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

func (s *Store) CreatePlan(ctx context.Context, plan contracts.PlanVersion, event contracts.EventEnvelope) error {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO plan_versions(plan_id,version,task_id,revision,parent_plan_id,reason,summary,plan_json,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, plan.PlanID, plan.Version, plan.TaskID, plan.Revision, nullIfEmpty(plan.ParentPlanID), plan.Reason, plan.Summary, string(planJSON), timestamp(plan.CreatedAt))
	if err == nil {
		// Insert every Step before its dependency edges: valid DAGs are not
		// required to be topologically ordered in the serialized PlanVersion.
		for _, step := range plan.Steps {
			spec, marshalErr := json.Marshal(step)
			if marshalErr != nil {
				err = marshalErr
				break
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO steps(step_id,plan_id,task_id,spec_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, step.StepID, plan.PlanID, plan.TaskID, string(spec), contracts.StepPending, now(), now())
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		for _, step := range plan.Steps {
			for _, dependency := range step.Dependencies {
				_, err = tx.ExecContext(ctx, `INSERT INTO step_dependencies(step_id,depends_on_step_id) VALUES(?,?)`, step.StepID, dependency)
				if err != nil {
					break
				}
			}
			if err != nil {
				break
			}
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

func (s *Store) GetLatestPlan(ctx context.Context, taskID string) (contracts.PlanVersion, error) {
	var serialized string
	err := s.db.QueryRowContext(ctx, `SELECT plan_json FROM plan_versions WHERE task_id=? ORDER BY revision DESC LIMIT 1`, taskID).Scan(&serialized)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.PlanVersion{}, contracts.NewError(contracts.ErrNotFound, "plan not found")
	}
	if err != nil {
		return contracts.PlanVersion{}, err
	}
	var plan contracts.PlanVersion
	err = json.Unmarshal([]byte(serialized), &plan)
	return plan, err
}

func (s *Store) GetSteps(ctx context.Context, planID string) ([]contracts.StepRuntime, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT step_id,plan_id,status,evidence_ids_json,artifact_ids_json,COALESCE(last_error_code,'') FROM steps WHERE plan_id=? ORDER BY step_id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []contracts.StepRuntime{}
	for rows.Next() {
		var state contracts.StepRuntime
		var evidence, artifacts string
		if err := rows.Scan(&state.StepID, &state.PlanID, &state.Status, &evidence, &artifacts, &state.LastErrorCode); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(evidence), &state.EvidenceIDs)
		_ = json.Unmarshal([]byte(artifacts), &state.ArtifactIDs)
		result = append(result, state)
	}
	return result, rows.Err()
}

func (s *Store) TransitionStep(ctx context.Context, stepID string, from, to contracts.StepStatus, event contracts.EventEnvelope) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE steps SET status=?,updated_at=? WHERE step_id=? AND status=?`, to, now(), stepID, from)
	if err == nil {
		var affected int64
		affected, _ = result.RowsAffected()
		if affected != 1 {
			err = contracts.NewError(contracts.ErrInvalidTransition, "step status changed concurrently or transition is invalid")
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

func (s *Store) SaveArtifact(ctx context.Context, artifact contracts.Artifact) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO artifacts(artifact_id,version,kind,media_type,storage_uri,content_hash,size_bytes,summary,workspace_relative_path,task_id,step_id,verified,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, artifact.ID, artifact.Version, artifact.Kind, artifact.MediaType, artifact.StorageURI, artifact.ContentHash, artifact.SizeBytes, artifact.Summary, nullIfEmpty(artifact.WorkspaceRelativePath), nullIfEmpty(artifact.TaskID), nullIfEmpty(artifact.StepID), boolToInt(artifact.Verified), timestamp(artifact.CreatedAt))
	return err
}

func (s *Store) SaveEvidence(ctx context.Context, evidence contracts.Evidence) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO evidence(evidence_id,kind,claim,artifact_id,location,verification_method,verified_at,confidence) VALUES(?,?,?,?,?,?,?,?)`, evidence.ID, evidence.Kind, evidence.Claim, evidence.ArtifactID, evidence.Location, evidence.VerificationMethod, timestamp(evidence.VerifiedAt), evidence.Confidence)
	return err
}

func (s *Store) AttachStepResults(ctx context.Context, stepID string, artifactIDs, evidenceIDs []string) error {
	artifacts, _ := json.Marshal(artifactIDs)
	evidence, _ := json.Marshal(evidenceIDs)
	_, err := s.db.ExecContext(ctx, `UPDATE steps SET artifact_ids_json=?, evidence_ids_json=?, updated_at=? WHERE step_id=?`, string(artifacts), string(evidence), now(), stepID)
	return err
}

func (s *Store) Events(ctx context.Context, taskID string, afterSequence int64) ([]contracts.EventEnvelope, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_id,event_version,event_type,aggregate_type,aggregate_id,COALESCE(run_id,''),sequence,timestamp,actor_type,actor_id,COALESCE(correlation_id,''),COALESCE(causation_id,''),payload_json FROM events WHERE aggregate_type='TASK' AND aggregate_id=? AND sequence>? ORDER BY sequence`, taskID, afterSequence)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []contracts.EventEnvelope{}
	for rows.Next() {
		var e contracts.EventEnvelope
		var payload, at string
		if err := rows.Scan(&e.EventID, &e.EventVersion, &e.EventType, &e.AggregateType, &e.AggregateID, &e.RunID, &e.Sequence, &at, &e.ActorType, &e.ActorID, &e.CorrelationID, &e.CausationID, &payload); err != nil {
			return nil, err
		}
		e.Timestamp, _ = parseTimestamp(at)
		if err := json.Unmarshal([]byte(payload), &e.Payload); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Store) SaveCheckpoint(ctx context.Context, id, taskID string, sequence int64, snapshot interface{}) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO checkpoints(checkpoint_id,version,task_id,sequence,snapshot_json,created_at) VALUES(?,?,?,?,?,?)`, id, contracts.SchemaVersion, taskID, sequence, string(data), now())
	return err
}

func (s *Store) NextSequence(ctx context.Context, aggregateType, aggregateID string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM events WHERE aggregate_type=? AND aggregate_id=?`, aggregateType, aggregateID).Scan(&seq)
	return seq, err
}

func (s *Store) appendEventTx(ctx context.Context, tx *sql.Tx, event contracts.EventEnvelope) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events(event_id,event_version,event_type,aggregate_type,aggregate_id,run_id,sequence,timestamp,actor_type,actor_id,correlation_id,causation_id,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.EventID, event.EventVersion, event.EventType, event.AggregateType, event.AggregateID, nullIfEmpty(event.RunID), event.Sequence, timestamp(event.Timestamp), event.ActorType, event.ActorID, nullIfEmpty(event.CorrelationID), nullIfEmpty(event.CausationID), string(payload))
	return err
}

func nullIfEmpty(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}
func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
