package main

import (
	"encoding/json"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/schemas"
)

func TestCheckpointCommandsBindTheOwnerQueue(t *testing.T) {
	job := store.Job{ProjectID: 1, ExecutionID: 2, TaskID: 3}
	page := store.CheckpointPage{Checkpoint: store.Checkpoint{Sequence: 1, At: time.Now().UTC()},
		OwnerInstructions: json.RawMessage(`[{"key":"first","body":"Add the title"}]`)}
	work, execution, batch := "WORK-3", "EXEC-2", "EXEC-2-CHECKPOINT-1"
	id := batch + "-COMMAND-first"
	message := schemas.AgentMessage{Version: "1.0", Project: "PROJECT-1", WorkItem: &work, Execution: &execution, ID: id, IdempotencyKey: id, Correlation: batch,
		Type: "COMMAND", Trust: "owner_authenticated", From: schemas.Actor{Type: "owner", ID: "owner"}, To: schemas.Actor{Type: "agent", ID: "worker"},
		Timestamp: page.At, Payload: map[string]any{"action": "queue_next", "checkpoint_sequence": 1, "key": "first", "body": "Add the title"}}
	check := func(m schemas.AgentMessage, valid bool) {
		t.Helper()
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		page.Commands = []json.RawMessage{raw}
		receipt, err := checkpointCommands(page, job, "worker")
		if (err == nil) != valid {
			t.Fatalf("valid=%v, error=%v", valid, err)
		}
		if valid {
			var rows []map[string]string
			if json.Unmarshal(receipt, &rows) != nil || len(rows) != 1 || rows[0]["message_id"] != id || rows[0]["body"] != "Add the title" {
				t.Fatal(string(receipt))
			}
		}
	}
	check(message, true)
	wrong := message
	wrong.To.ID = "other"
	check(wrong, false)
	wrong = message
	wrong.Trust = "agent_generated"
	check(wrong, false)
	job.GoalContext = json.RawMessage(`{"goal_id":7}`)
	check(message, false)
	goal := "GOAL-7"
	message.Goal = &goal
	check(message, true)
	message.Payload["body"] = "Different title"
	check(message, false)
	page.Commands = nil
	if _, err := checkpointCommands(page, job, "worker"); err == nil {
		t.Fatal("accepted instructions without ACP commands")
	}
}
