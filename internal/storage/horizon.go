package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/xm/simplenessagent/pkg/contracts"
)

// SaveHorizon materializes the latest long-horizon state and appends the
// corresponding audit event in the same transaction.
func (s *Store) SaveHorizon(ctx context.Context, state contracts.HorizonState, event contracts.EventEnvelope) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO task_horizons(horizon_id,task_id,version,state_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(horizon_id) DO UPDATE SET version=excluded.version,state_json=excluded.state_json,updated_at=excluded.updated_at`,
		state.HorizonID, state.TaskID, state.Version, string(encoded), timestamp(state.StartedAt), timestamp(state.UpdatedAt))
	if err == nil {
		err = s.appendEventTx(ctx, tx, event)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) GetHorizon(ctx context.Context, taskID string) (contracts.HorizonState, error) {
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT state_json FROM task_horizons WHERE task_id=?`, taskID).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.HorizonState{}, contracts.NewError(contracts.ErrNotFound, "long-horizon state not found")
	}
	if err != nil {
		return contracts.HorizonState{}, err
	}
	var state contracts.HorizonState
	if err = json.Unmarshal([]byte(encoded), &state); err != nil {
		return contracts.HorizonState{}, err
	}
	return state, nil
}

func (s *Store) UpsertModelRoleProfile(ctx context.Context, profile contracts.ModelRoleProfile) error {
	encoded, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO model_role_profiles(profile_id,deployment_id,role,version,profile_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(deployment_id,role) DO UPDATE SET profile_id=excluded.profile_id,version=excluded.version,profile_json=excluded.profile_json,updated_at=excluded.updated_at`,
		profile.ID, profile.DeploymentID, profile.Role, profile.Version, string(encoded), timestamp(profile.CreatedAt), timestamp(profile.UpdatedAt))
	return err
}

func (s *Store) GetModelRoleProfile(ctx context.Context, deploymentID string, role contracts.ModelRole) (contracts.ModelRoleProfile, error) {
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT profile_json FROM model_role_profiles WHERE deployment_id=? AND role=?`, deploymentID, role).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ModelRoleProfile{}, contracts.NewError(contracts.ErrNotFound, "model role profile not found")
	}
	if err != nil {
		return contracts.ModelRoleProfile{}, err
	}
	var profile contracts.ModelRoleProfile
	err = json.Unmarshal([]byte(encoded), &profile)
	return profile, err
}
