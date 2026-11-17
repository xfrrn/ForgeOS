package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"forgeos/internal/store"
	"forgeos/schemas"
)

func checkpointAnswer(raw []json.RawMessage, workerID string, job store.Job, sequence int, q nativeQuestion) (json.RawMessage, error) {
	if len(raw) != 2 || job.ProjectID <= 0 || job.Spec == nil {
		return nil, errors.New("expected a checkpoint decision request and response")
	}
	var goal *string
	if len(job.GoalContext) > 0 {
		var context struct {
			ID int64 `json:"goal_id"`
		}
		if err := json.Unmarshal(job.GoalContext, &context); err != nil || context.ID <= 0 {
			return nil, errors.New("invalid goal in decision message scope")
		}
		id := fmt.Sprintf("GOAL-%d", context.ID)
		goal = &id
	}
	var pair [2]schemas.AgentMessage
	for i, encoded := range raw {
		if err := schemas.ValidateAgentMessage(encoded); err != nil {
			return nil, fmt.Errorf("invalid checkpoint decision message: %w", err)
		}
		m := &pair[i]
		if err := json.Unmarshal(encoded, m); err != nil {
			return nil, err
		}
		if m.Project != fmt.Sprintf("PROJECT-%d", job.ProjectID) || m.WorkItem == nil || *m.WorkItem != fmt.Sprintf("WORK-%d", job.TaskID) ||
			m.Execution == nil || *m.Execution != fmt.Sprintf("EXEC-%d", job.ExecutionID) || (goal == nil) != (m.Goal == nil) || goal != nil && *goal != *m.Goal {
			return nil, errors.New("decision message belongs to another task or execution")
		}
	}
	request, response := pair[0], pair[1]
	owner := schemas.Actor{Type: "owner", ID: "owner"}
	agent := schemas.Actor{Type: "agent", ID: workerID}
	refs, _ := request.Payload["evidence_refs"].([]any)
	if request.Type != "REQUEST" || request.From != agent || request.To != owner || request.Trust != "agent_generated" ||
		request.Payload["request_kind"] != "decision" || request.Payload["blocking"] != true || request.Payload["checkpoint_sequence"] != float64(sequence) ||
		request.Payload["question"] != q.Question || request.Payload["evidence"] != q.Evidence || len(refs) != 1 || refs[0] != "git:"+job.Spec.BaseCommit {
		return nil, errors.New("decision request does not match the waiting checkpoint")
	}
	answer, ok := response.Payload["answer"].(string)
	if response.Type != "RESPONSE" || response.From != owner || response.To != agent || response.Trust != "owner_authenticated" ||
		response.Correlation != request.Correlation || response.Payload["request_id"] != request.ID || response.ID == request.ID ||
		response.IdempotencyKey == request.IdempotencyKey || !ok || strings.TrimSpace(answer) == "" || len(answer) > 4096 {
		return nil, errors.New("decision response does not match the owner request")
	}
	return json.Marshal(map[string]string{"question": q.Question, "answer": answer, "request_id": request.ID, "response_id": response.ID, "correlation_id": request.Correlation})
}
