// Package eventstore creates immutable, ordered EventEnvelope values.
package eventstore

import (
	"context"
	"time"

	"github.com/xm/simplenessagent/internal/storage"
	"github.com/xm/simplenessagent/internal/task"
	"github.com/xm/simplenessagent/pkg/contracts"
)

func NewTaskEvent(ctx context.Context, store *storage.Store, taskID, eventType string, payload map[string]interface{}) (contracts.EventEnvelope, error) {
	sequence, err := store.NextSequence(ctx, "TASK", taskID)
	if err != nil {
		return contracts.EventEnvelope{}, err
	}
	return contracts.EventEnvelope{
		EventID: task.NewID("evt"), EventVersion: contracts.SchemaVersion, EventType: eventType,
		AggregateType: "TASK", AggregateID: taskID, Sequence: sequence, Timestamp: time.Now().UTC(),
		ActorType: "CORE", ActorID: "simpleness-core", Payload: payload,
	}, nil
}
