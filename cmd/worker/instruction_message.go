package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"forgeos/internal/store"
	"forgeos/schemas"
)

func checkpointCommands(page store.CheckpointPage, job store.Job, worker string) (json.RawMessage, error) {
	if len(page.OwnerInstructions) == 0 && len(page.Commands) == 0 {
		return nil, nil
	}
	var queued []store.OwnerInstruction
	if err := json.Unmarshal(page.OwnerInstructions, &queued); err != nil || len(queued) == 0 || len(queued) > 8 || len(queued) != len(page.Commands) {
		return nil, errors.New("checkpoint commands do not match the queued instructions")
	}
	type receipt struct {
		store.OwnerInstruction
		MessageID   string `json:"message_id"`
		Correlation string `json:"correlation_id"`
	}
	batch := fmt.Sprintf("EXEC-%d-CHECKPOINT-%d", job.ExecutionID, page.Sequence)
	seen := map[string]bool{}
	receipts := make([]receipt, 0, len(queued))
	for i, encoded := range page.Commands {
		m, err := readScopedMessage(encoded, job)
		if err != nil {
			return nil, err
		}
		item := queued[i]
		if !item.Valid() || seen[item.Key] || m.Type != "COMMAND" || m.Trust != "owner_authenticated" ||
			m.From != (schemas.Actor{Type: "owner", ID: "owner"}) || m.To != (schemas.Actor{Type: "agent", ID: worker}) ||
			m.Correlation != batch || m.ID != batch+"-COMMAND-"+item.Key || m.IdempotencyKey != m.ID || !m.Timestamp.Equal(page.At) ||
			m.Payload["action"] != "queue_next" || m.Payload["checkpoint_sequence"] != float64(page.Sequence) ||
			m.Payload["key"] != item.Key || m.Payload["body"] != item.Body {
			return nil, errors.New("unexpected Owner command at this checkpoint")
		}
		seen[item.Key] = true
		receipts = append(receipts, receipt{item, m.ID, m.Correlation})
	}
	return json.Marshal(receipts)
}
