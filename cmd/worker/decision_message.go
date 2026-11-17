package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"forgeos/internal/store"
	"forgeos/schemas"
)

func readScopedMessage(encoded json.RawMessage, job store.Job) (schemas.AgentMessage, error) {
	var m schemas.AgentMessage
	if err := schemas.ValidateAgentMessage(encoded); err != nil {
		return m, fmt.Errorf("invalid checkpoint message: %w", err)
	}
	if err := json.Unmarshal(encoded, &m); err != nil {
		return m, err
	}
	var goal *string
	if len(job.GoalContext) > 0 {
		var context struct {
			ID int64 `json:"goal_id"`
		}
		if err := json.Unmarshal(job.GoalContext, &context); err != nil || context.ID <= 0 {
			return m, errors.New("invalid goal in checkpoint message scope")
		}
		id := fmt.Sprintf("GOAL-%d", context.ID)
		goal = &id
	}
	if job.ProjectID <= 0 || m.Project != fmt.Sprintf("PROJECT-%d", job.ProjectID) || m.WorkItem == nil || *m.WorkItem != fmt.Sprintf("WORK-%d", job.TaskID) ||
		m.Execution == nil || *m.Execution != fmt.Sprintf("EXEC-%d", job.ExecutionID) || (goal == nil) != (m.Goal == nil) || goal != nil && *goal != *m.Goal {
		return m, errors.New("checkpoint message belongs to another task or execution")
	}
	return m, nil
}

func checkpointAnswer(raw []json.RawMessage, workerID string, job store.Job, sequence int, q nativeQuestion) (json.RawMessage, error) {
	if len(raw) != 2 || job.ProjectID <= 0 || job.Spec == nil {
		return nil, errors.New("expected a checkpoint decision request and response")
	}
	var pair [2]schemas.AgentMessage
	for i, encoded := range raw {
		message, err := readScopedMessage(encoded, job)
		if err != nil {
			return nil, err
		}
		pair[i] = message
	}
	request, response := pair[0], pair[1]
	owner := schemas.Actor{Type: "owner", ID: "owner"}
	agent := schemas.Actor{Type: "agent", ID: workerID}
	recipient, kind := owner, "decision"
	manager := schemas.Actor{Type: "agent", ID: "management"}
	reference := fmt.Sprintf("decision:%d", q.ReferenceDecisionID)
	if q.ReferenceDecisionID > 0 {
		if !job.DecisionLookups || request.Payload["reference"] != reference {
			return nil, errors.New("unexpected project decision lookup")
		}
		recipient, kind = manager, "decision_lookup"
	}
	refs, _ := request.Payload["evidence_refs"].([]any)
	if request.Type != "REQUEST" || request.From != agent || request.To != recipient || request.Trust != "agent_generated" ||
		request.Payload["request_kind"] != kind || request.Payload["blocking"] != true || request.Payload["checkpoint_sequence"] != float64(sequence) ||
		request.Payload["question"] != q.Question || request.Payload["evidence"] != q.Evidence || len(refs) != 1 || refs[0] != "git:"+job.Spec.BaseCommit {
		return nil, errors.New("decision request does not match the waiting checkpoint")
	}
	answer, ok := response.Payload["answer"].(string)
	if response.Type != "RESPONSE" || response.To != agent ||
		response.Correlation != request.Correlation || response.Payload["request_id"] != request.ID || response.ID == request.ID ||
		response.IdempotencyKey == request.IdempotencyKey || !ok || strings.TrimSpace(answer) == "" || len(answer) > 4096 {
		return nil, errors.New("decision response does not match the owner request")
	}
	receipt := map[string]any{"question": q.Question, "answer": answer, "request_id": request.ID, "response_id": response.ID, "correlation_id": request.Correlation}
	if response.From == manager && q.ReferenceDecisionID > 0 && response.Trust == "agent_generated" {
		decision, _ := response.Payload["decision"].(map[string]any)
		session, _ := response.Payload["session_id"].(string)
		validity, _ := decision["validity"].(string)
		if session == "" || validity == "" || decision["id"] != reference || decision["text"] != answer {
			return nil, errors.New("invalid Management decision evidence")
		}
		receipt["answered_by"], receipt["decision"], receipt["session_id"] = "management", decision, session
	} else if response.From != owner || response.Trust != "owner_authenticated" {
		return nil, errors.New("invalid checkpoint decision respondent")
	}
	return json.Marshal(receipt)
}
