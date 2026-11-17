package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
	"forgeos/schemas"
)

func TestCheckpointAnswerProtocolNegotiation(t *testing.T) {
	q := nativeQuestion{Question: "Which heading?", Evidence: "Two choices."}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input store.CheckpointQuestion
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Question != q.Question {
			t.Error("invalid question", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"answer": "legacy reply"})
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client()}
	for _, protocol := range []string{"", "1.0", "unknown"} {
		reply, err := w.waitForOwner(context.Background(), store.Job{DecisionProtocol: protocol}, 0, "native", q, nil)
		if protocol == "" && (err != nil || len(reply) == 0) || protocol != "" && (err == nil || reply != nil) {
			t.Fatal(protocol, string(reply), err)
		}
	}
}

func TestCheckpointAnswerMatchesClaimAndQuestion(t *testing.T) {
	job := store.Job{ProjectID: 12, TaskID: 7, ExecutionID: 9, GoalContext: json.RawMessage(`{"goal_id":4}`), Spec: &store.IssueSpec{BaseCommit: "base"}}
	goal, task, exec := "GOAL-4", "WORK-7", "EXEC-9"
	q := nativeQuestion{Question: "Which heading?", Evidence: "Two choices."}
	agent := schemas.Actor{Type: "agent", ID: "node"}
	owner := schemas.Actor{Type: "owner", ID: "owner"}
	newPair := func() []schemas.AgentMessage {
		request := schemas.AgentMessage{Version: "1.0", ID: "req", Correlation: "group", Project: "PROJECT-12", Goal: &goal, WorkItem: &task, Execution: &exec,
			From: agent, To: owner, Type: "REQUEST", Timestamp: time.Now(), IdempotencyKey: "req", Trust: "agent_generated",
			Payload: map[string]any{"request_kind": "decision", "blocking": true, "checkpoint_sequence": 3, "question": q.Question, "evidence": q.Evidence, "evidence_refs": []string{"git:base"}}}
		reply := request
		reply.Type, reply.ID, reply.From, reply.To, reply.Trust, reply.IdempotencyKey = "RESPONSE", "rep", owner, agent, "owner_authenticated", "rep"
		reply.Payload = map[string]any{"request_id": "req", "answer": "Harbor"}
		return []schemas.AgentMessage{request, reply}
	}
	read := func(pair []schemas.AgentMessage) (json.RawMessage, error) {
		var raw []json.RawMessage
		for _, m := range pair {
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, b)
		}
		return checkpointAnswer(raw, "node", job, 3, q)
	}
	got, err := read(newPair())
	var answer map[string]string
	if err != nil || json.Unmarshal(got, &answer) != nil || answer["answer"] != "Harbor" || answer["request_id"] != "req" || answer["response_id"] != "rep" || answer["correlation_id"] != "group" {
		t.Fatal(string(got), err)
	}
	for name, change := range map[string]func([]schemas.AgentMessage){
		"project":     func(m []schemas.AgentMessage) { m[0].Project = "PROJECT-2" },
		"goal":        func(m []schemas.AgentMessage) { m[1].Goal = nil },
		"work item":   func(m []schemas.AgentMessage) { m[1].WorkItem = m[0].Goal },
		"execution":   func(m []schemas.AgentMessage) { m[0].Execution = nil },
		"sequence":    func(m []schemas.AgentMessage) { m[0].Payload["checkpoint_sequence"] = 4 },
		"question":    func(m []schemas.AgentMessage) { m[0].Payload["question"] = "Other question" },
		"base":        func(m []schemas.AgentMessage) { m[0].Payload["evidence_refs"] = []string{"git:other"} },
		"owner":       func(m []schemas.AgentMessage) { m[1].From = agent },
		"recipient":   func(m []schemas.AgentMessage) { m[1].To = owner },
		"source":      func(m []schemas.AgentMessage) { m[1].Trust = "agent_generated" },
		"correlation": func(m []schemas.AgentMessage) { m[1].Correlation = "other" },
		"parent":      func(m []schemas.AgentMessage) { m[1].Payload["request_id"] = "other" },
		"key":         func(m []schemas.AgentMessage) { m[1].IdempotencyKey = m[0].IdempotencyKey },
		"schema":      func(m []schemas.AgentMessage) { m[0].Version = "2.0" },
	} {
		t.Run(name, func(t *testing.T) {
			m := newPair()
			change(m)
			if _, err := read(m); err == nil {
				t.Fatal("accepted mismatched message")
			}
		})
	}
	manager := schemas.Actor{Type: "agent", ID: "management"}
	q.read(`{"forgeos_lookup_decision":13}`)
	if !q.requested || q.ReferenceDecisionID != 13 || q.Question == "" {
		t.Fatal(q)
	}
	pair := newPair()
	pair[0].To = manager
	pair[0].Payload["request_kind"], pair[0].Payload["reference"] = "decision_lookup", "decision:13"
	pair[1].From, pair[1].Trust = manager, "agent_generated"
	pair[1].Payload["session_id"] = "reviewer"
	pair[1].Payload["decision"] = map[string]any{"id": "decision:13", "validity": "Documentation", "text": "Harbor"}
	if _, err := read(pair); err == nil {
		t.Fatal("unnegotiated lookup")
	}
	job.DecisionLookups = true
	got, err = read(pair)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := issuePrompt(job, "", store.CheckpointPage{}, got)
	if err != nil || !strings.Contains(prompt, `"management_reference":`) || strings.Contains(prompt, `"owner_clarification":`) {
		t.Fatal(prompt, err)
	}
	pair[1].Payload["answer"] = "Different"
	if _, err := read(pair); err == nil {
		t.Fatal("accepted a fabricated statement")
	}
	q.read(`{"forgeos_lookup_decision":13,"forgeos_question":"approve?"}`)
	if !q.requested || q.Question != "" {
		t.Fatal("ambiguous lookup", q)
	}

}
