package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func (s *Store) CreateDeployment(ctx context.Context, item contracts.Deployment) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO deployments(deployment_id,version,name,provider_type,location,endpoint,credential_ref,model,runtime,runtime_version,quantization,model_profile_id,capability_snapshot_id,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.Version, item.Name, item.ProviderType, item.Location, item.Endpoint, nullIfEmpty(item.CredentialRef), item.Model, nullIfEmpty(item.Runtime), nullIfEmpty(item.RuntimeVersion), nullIfEmpty(item.Quantization), nullIfEmpty(item.ModelProfileID), nullIfEmpty(item.CapabilitySnapshotID), boolToInt(item.Enabled), timestamp(item.CreatedAt), timestamp(item.UpdatedAt))
	return err
}

func (s *Store) GetDeployment(ctx context.Context, id string) (contracts.Deployment, error) {
	var item contracts.Deployment
	var credential, runtime, runtimeVersion, quantization, profile, capability sql.NullString
	var enabled int
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT deployment_id,version,name,provider_type,location,endpoint,credential_ref,model,runtime,runtime_version,quantization,model_profile_id,capability_snapshot_id,enabled,created_at,updated_at FROM deployments WHERE deployment_id=?`, id).Scan(&item.ID, &item.Version, &item.Name, &item.ProviderType, &item.Location, &item.Endpoint, &credential, &item.Model, &runtime, &runtimeVersion, &quantization, &profile, &capability, &enabled, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return item, contracts.NewError(contracts.ErrNotFound, "deployment not found")
	}
	if err != nil {
		return item, err
	}
	item.CredentialRef, item.Runtime, item.RuntimeVersion, item.Quantization, item.ModelProfileID, item.CapabilitySnapshotID = credential.String, runtime.String, runtimeVersion.String, quantization.String, profile.String, capability.String
	item.Enabled = enabled != 0
	item.CreatedAt, _ = parseTimestamp(created)
	item.UpdatedAt, _ = parseTimestamp(updated)
	return item, nil
}

func (s *Store) ListDeployments(ctx context.Context) ([]contracts.Deployment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT deployment_id FROM deployments ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []contracts.Deployment{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := s.GetDeployment(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SaveCapabilitySnapshot(ctx context.Context, snapshot contracts.CapabilitySnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO deployment_capabilities(capability_snapshot_id,deployment_id,version,snapshot_json,created_at) VALUES(?,?,?,?,?)`, snapshot.ID, snapshot.DeploymentID, snapshot.Version, string(data), timestamp(snapshot.ProbedAt))
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE deployments SET capability_snapshot_id=?,updated_at=? WHERE deployment_id=?`, snapshot.ID, now(), snapshot.DeploymentID)
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) GetCapabilitySnapshot(ctx context.Context, id string) (contracts.CapabilitySnapshot, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT snapshot_json FROM deployment_capabilities WHERE capability_snapshot_id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.CapabilitySnapshot{}, contracts.NewError(contracts.ErrNotFound, "capability snapshot not found")
	}
	if err != nil {
		return contracts.CapabilitySnapshot{}, err
	}
	var snapshot contracts.CapabilitySnapshot
	err = json.Unmarshal([]byte(data), &snapshot)
	return snapshot, err
}
