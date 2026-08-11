package storage

import (
	"context"
	"database/sql"
	"errors"

	"github.com/xm/simplenessagent/pkg/contracts"
)

func (s *Store) ListTaskEvidence(ctx context.Context, taskID string) ([]contracts.Evidence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.evidence_id,e.kind,e.claim,e.artifact_id,e.location,e.verification_method,e.verified_at,e.confidence FROM evidence e JOIN artifacts a ON a.artifact_id=e.artifact_id WHERE a.task_id=? ORDER BY e.verified_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []contracts.Evidence{}
	for rows.Next() {
		var value contracts.Evidence
		var verified string
		if err := rows.Scan(&value.ID, &value.Kind, &value.Claim, &value.ArtifactID, &value.Location, &value.VerificationMethod, &verified, &value.Confidence); err != nil {
			return nil, err
		}
		value.VerifiedAt, _ = parseTimestamp(verified)
		result = append(result, value)
	}
	if err = rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return result, nil
}
