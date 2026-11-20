package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/store"
)

const (
	managementChatRole       = "management-chat"
	managementChatResultPath = "/internal/management-chat/result"
)

const managementChatSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["answer", "citations", "action"],
  "properties": {
    "answer": {"type": "string"},
    "citations": {"type": "array", "items": {"type": "string"}},
    "action": {
      "anyOf": [
        {"type": "null"},
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["kind", "issue_number"],
          "properties": {
            "kind": {"type": "string", "enum": ["dispatch_issue", "check_head"]},
            "issue_number": {"type": ["integer", "null"]}
          }
        }
      ]
    }
  }
}`

type managementChatJob struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"project_id"`
	Lease      string    `json:"lease"`
	LeaseUntil time.Time `json:"lease_until"`
	Prompt     string    `json:"prompt"`
}

type managementChatAction struct {
	Kind        string `json:"kind"`
	IssueNumber *int64 `json:"issue_number,omitempty"`
}

type managementChatAnswer struct {
	Answer    string                `json:"answer"`
	Citations []string              `json:"citations"`
	Action    *managementChatAction `json:"action"`
}

type managementChatResult struct {
	ID    int64  `json:"id"`
	Lease string `json:"lease"`
	managementChatAnswer
	Error string `json:"error,omitempty"`
}

func (w *worker) runManagementChat(ctx context.Context) error {
	for ctx.Err() == nil {
		if err := w.replayRoleReceipts(ctx, managementChatRole); err != nil {
			return err
		}
		var job managementChatJob
		status, err := w.claimRole(ctx, managementChatRole, "/internal/management-chat/claim?agent_settings=1&supports_role_resume=1", struct{}{}, &job)
		if status == http.StatusNoContent || temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("management chat claim failed (HTTP %d)", status)
		}
		if err := w.answerManagementChat(ctx, job); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (w *worker) answerManagementChat(ctx context.Context, job managementChatJob) error {
	result := managementChatResult{ID: job.ID, Lease: job.Lease}
	if job.ID <= 0 || job.ProjectID <= 0 || job.LeaseUntil.IsZero() || strings.TrimSpace(job.Prompt) == "" {
		return fmt.Errorf("invalid management chat assignment")
	}
	prompt := agents.Builtin("management-chat") + "\n\n" + job.Prompt
	turnErr := context.DeadlineExceeded
	if time.Now().Before(job.LeaseUntil) {
		turn, cancel := managementTurn(ctx, &job.LeaseUntil)
		_, turnErr = w.roleJSONTurn(turn, true, store.ManagementProcessReport{Lease: job.Lease}, managementChatSchema, prompt, 32<<10, &result.managementChatAnswer)
		cancel()
	}
	if ctx.Err() != nil {
		// Keep the assignment for recovery when the node is stopping.
		return ctx.Err()
	}
	result.Answer = strings.TrimSpace(result.Answer)
	for _, secret := range outputSecrets(job.Lease) {
		result.Answer = strings.ReplaceAll(result.Answer, secret, "[已隐藏]")
	}
	if turnErr != nil || result.Answer == "" || len(result.Answer) > 16<<10 || strings.ContainsRune(result.Answer, 0) {
		result.fail()
	}
	if len(result.Citations) > 24 {
		result.Citations = result.Citations[:24]
	}
	if result.Citations == nil {
		result.Citations = []string{}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(raw) > 64<<10 {
		result.fail()
		raw, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}
	status, err := w.roleReceipt(ctx, managementChatResultPath, json.RawMessage(raw))
	if err != nil {
		return err
	}
	log.Printf("management chat job %d (HTTP %d)", job.ID, status)
	return nil
}

func (r *managementChatResult) fail() {
	r.managementChatAnswer = managementChatAnswer{Citations: []string{}}
	r.Error = "本次未能生成项目回复，请重试。"
}
