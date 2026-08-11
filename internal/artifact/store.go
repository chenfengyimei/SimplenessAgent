// Package artifact provides atomic, content-addressed storage for long outputs.
package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/pkg/contracts"
)

type Store struct{ root string }

func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) Put(kind, mediaType, summary, taskID, stepID string, content []byte) (contracts.Artifact, error) {
	digest := sha256.Sum256(content)
	hash := hex.EncodeToString(digest[:])
	directory := filepath.Join(s.root, hash[:2])
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return contracts.Artifact{}, err
	}
	finalPath := filepath.Join(directory, hash)
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		temporary, err := os.CreateTemp(directory, ".pending-*")
		if err != nil {
			return contracts.Artifact{}, err
		}
		name := temporary.Name()
		if _, err = temporary.Write(content); err == nil {
			err = temporary.Sync()
		}
		if closeErr := temporary.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(name)
			return contracts.Artifact{}, err
		}
		if err = os.Rename(name, finalPath); err != nil {
			if !os.IsExist(err) {
				_ = os.Remove(name)
				return contracts.Artifact{}, err
			}
		}
	}
	return contracts.Artifact{ID: task.NewID("art"), Version: contracts.SchemaVersion, Kind: kind, MediaType: mediaType, StorageURI: "artifact://sha256/" + hash, ContentHash: "sha256:" + hash, SizeBytes: int64(len(content)), Summary: summary, TaskID: taskID, StepID: stepID, Verified: true, CreatedAt: time.Now().UTC()}, nil
}

func (s *Store) Read(artifact contracts.Artifact) ([]byte, error) {
	const prefix = "artifact://sha256/"
	if !strings.HasPrefix(artifact.StorageURI, prefix) {
		return nil, contracts.NewError(contracts.ErrArtifactCorrupt, "unsupported artifact URI")
	}
	hash := strings.TrimPrefix(artifact.StorageURI, prefix)
	if len(hash) < 2 {
		return nil, contracts.NewError(contracts.ErrArtifactCorrupt, "invalid artifact hash")
	}
	content, err := os.ReadFile(filepath.Join(s.root, hash[:2], hash))
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != hash {
		return nil, contracts.NewError(contracts.ErrArtifactCorrupt, "artifact hash mismatch")
	}
	return content, nil
}

func (s *Store) Root() string { return s.root }
