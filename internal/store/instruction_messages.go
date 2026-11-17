package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"forgeos/schemas"
)

// The receiving checkpoint freezes the preceding queue. Its timestamp survives retries.
func (s *Store) InstructionMessages(ctx context.Context, task, execution int64, sequence int) ([]json.RawMessage, error) {
	var project int64
	var goal sql.NullInt64
	var worker string
	var at time.Time
	var items []byte
	err := s.db.QueryRowContext(ctx, `SELECT t.project_id,t.goal_id,e.worker,c.at,p.next_instructions
		FROM execution_checkpoints c JOIN executions e ON e.id=c.execution_id JOIN work_items t ON t.id=e.task_id
		LEFT JOIN execution_checkpoints p ON p.execution_id=c.execution_id AND p.sequence=c.sequence-1
		WHERE t.id=$1 AND e.id=$2 AND c.sequence=$3`, task, execution, sequence).Scan(&project, &goal, &worker, &at, &items)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var queued []OwnerInstruction
	if items != nil {
		if err := json.Unmarshal(items, &queued); err != nil {
			return nil, err
		}
	}
	messages := make([]json.RawMessage, 0, len(queued))
	workID, execID := fmt.Sprintf("WORK-%d", task), fmt.Sprintf("EXEC-%d", execution)
	batch := fmt.Sprintf("%s-CHECKPOINT-%d", execID, sequence)
	m := schemas.AgentMessage{Version: "1.0", Correlation: batch, Project: fmt.Sprintf("PROJECT-%d", project),
		WorkItem: &workID, Execution: &execID, From: schemas.Actor{Type: "owner", ID: "owner"}, To: schemas.Actor{Type: "agent", ID: worker},
		Type: "COMMAND", Trust: "owner_authenticated", Timestamp: at.UTC()}
	if goal.Valid {
		id := fmt.Sprintf("GOAL-%d", goal.Int64)
		m.Goal = &id
	}
	for _, instruction := range queued {
		m.ID = batch + "-COMMAND-" + instruction.Key
		m.IdempotencyKey = m.ID
		m.Payload = map[string]any{"action": "queue_next", "checkpoint_sequence": sequence, "key": instruction.Key, "body": instruction.Body}
		raw, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		if err := schemas.ValidateAgentMessage(raw); err != nil {
			return nil, err
		}
		messages = append(messages, raw)
	}
	return messages, nil
}
