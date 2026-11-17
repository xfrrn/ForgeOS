package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"forgeos/schemas"
)

const questionMessageSource = `SELECT t.project_id,t.goal_id,to_jsonb(q)
	FROM work_items t JOIN decision_questions q ON q.task_id=t.id
	WHERE q.checkpoint_sequence IS NOT NULL AND `

func (s *Store) QuestionMessages(ctx context.Context, taskID, questionID int64) ([]schemas.AgentMessage, error) {
	return readQuestionMessages(s.db.QueryRowContext(ctx, questionMessageSource+"q.task_id=$1 AND q.id=$2", taskID, questionID))
}

func (s *Store) CheckpointMessages(ctx context.Context, executionID int64, sequence int) ([]schemas.AgentMessage, error) {
	return readQuestionMessages(s.db.QueryRowContext(ctx, questionMessageSource+"q.execution_id=$1 AND q.checkpoint_sequence=$2", executionID, sequence))
}

// Use the original question and answer receipts, including for executions predating ACP.
func readQuestionMessages(row *sql.Row) ([]schemas.AgentMessage, error) {
	var project int64
	var goal sql.NullInt64
	var raw []byte
	err := row.Scan(&project, &goal, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var q struct {
		DecisionQuestion
		TaskID int64 `json:"task_id"`
	}
	if err = json.Unmarshal(raw, &q); err != nil {
		return nil, err
	}
	work := "WORK-" + strconv.FormatInt(q.TaskID, 10)
	execID := "EXEC-" + strconv.FormatInt(q.ExecutionID, 10)
	correlation := "QUESTION-" + strconv.FormatInt(q.ID, 10)
	request := schemas.AgentMessage{Version: "1.0", ID: correlation + "-REQUEST", Correlation: correlation,
		Project: "PROJECT-" + strconv.FormatInt(project, 10), WorkItem: &work, Execution: &execID,
		From: schemas.Actor{Type: "agent", ID: q.Requester}, To: schemas.Actor{Type: "owner", ID: "owner"},
		Type: "REQUEST", Trust: "agent_generated", Timestamp: q.At.UTC(),
		Payload: map[string]any{"request_kind": "decision", "question": q.Question, "blocking": true, "evidence": q.Evidence,
			"evidence_refs": []string{"git:" + q.Commit}, "checkpoint_sequence": *q.Sequence},
	}
	if goal.Valid {
		id := "GOAL-" + strconv.FormatInt(goal.Int64, 10)
		request.Goal = &id
	}
	request.IdempotencyKey = request.ID
	messages := []schemas.AgentMessage{request}
	if q.AnsweredAt != nil {
		reply := request
		reply.ID = correlation + "-RESPONSE"
		reply.IdempotencyKey = reply.ID
		reply.From, reply.To = request.To, request.From
		reply.From.ID = q.AnsweredBy
		reply.Type, reply.Trust, reply.Timestamp = "RESPONSE", "owner_authenticated", q.AnsweredAt.UTC()
		reply.Payload = map[string]any{"request_id": request.ID, "answer": q.Answer}
		messages = append(messages, reply)
	}
	for _, m := range messages {
		raw, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		if err = schemas.ValidateAgentMessage(raw); err != nil {
			return nil, err
		}
	}
	return messages, nil
}
