package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"forgeos/internal/agents"
	"forgeos/internal/chatkernel"
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
        },
        {
          "type": "object",
          "additionalProperties": false,
          "required": ["kind","title","body","work_type"],
          "properties": {
            "kind": {"type":"string","enum":["create_issue"]},
            "title": {"type":"string"},
            "body": {"type":"string"},
            "work_type": {"type":"string","enum":["development","code-review","testing","investigation","operations"]}
          }
        }
      ]
    }
  }
}`

type managementChatJob struct {
	Native     *store.FeatureClient `json:"native,omitempty"`
	FeatureID  int64                `json:"feature_id,omitempty"`
	ID         int64                `json:"id"`
	ProjectID  int64                `json:"project_id"`
	Lease      string               `json:"lease"`
	LeaseUntil time.Time            `json:"lease_until"`
	Prompt     string               `json:"prompt"`
}

type managementChatAction struct {
	Title       string `json:"title,omitempty"`
	Body        string `json:"body,omitempty"`
	WorkType    string `json:"work_type,omitempty"`
	Kind        string `json:"kind"`
	IssueNumber *int64 `json:"issue_number,omitempty"`
}

type chatProjectKey struct{}

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
	nativeParam := ""
	if os.Getenv("FORGEOS_FEATURE_CLIENTS_FILE") != "" {
		nativeParam = "&feature_native=1"
		reportCtx, stop := context.WithCancel(ctx)
		defer stop()
		go w.featureClientHeartbeat(reportCtx)
	}
	for ctx.Err() == nil {
		if err := w.replayRoleReceipts(ctx, managementChatRole); err != nil {
			return err
		}
		var job managementChatJob
		status, err := w.claimRole(ctx, managementChatRole, "/internal/management-chat/claim?project_kernel=1&agent_settings=1&supports_role_resume=1"+nativeParam, struct{}{}, &job)
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
	var providerFailure, providerKey string
	if time.Now().Before(job.LeaseUntil) {
		turn, cancel := managementTurn(ctx, &job.LeaseUntil)
		if job.Native != nil {
			result.Answer, turnErr = w.nativeFeatureReply(turn, job)
			if turnErr != nil {
				providerFailure = "native_reply_unconfirmed"
				log.Printf("feature chat %d: %v", job.ID, turnErr)
			}
		} else {
			var provider chatkernel.Provider
			status, err := w.post(turn, "/internal/management-chat/provider", map[string]any{"id": job.ID, "lease": job.Lease}, &provider)
			if err == nil && status == 200 && w.roleHarness("codex") == "codex" {
				providerKey = provider.APIKey
				cfg, err := chatkernel.ForProject(os.Getenv("FORGEOS_CHAT_KERNEL_CONFIG"), job.ProjectID, provider)
				if err != nil {
					turnErr = err
				} else {
					cfg.TaskTool = func(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
						var out json.RawMessage
						status, err := w.post(ctx, "/internal/management-chat/task-tool", map[string]any{"id": job.ID, "lease": job.Lease, "name": name, "arguments": args}, &out)
						if err != nil || status != 200 {
							return nil, fmt.Errorf("project task lookup unavailable (HTTP %d)", status)
						}
						return out, nil
					}
					cfg.Preview = w.chatPreview(turn, job, append(outputSecrets(job.Lease), provider.APIKey))
					turn = context.WithValue(turn, chatProjectKey{}, cfg)
					_, turnErr = w.roleJSONTurn(turn, true, store.ManagementProcessReport{Lease: job.Lease}, managementChatSchema, prompt, 32<<10, &result.managementChatAnswer)
					providerFailure = cfg.FailureCode()
				}
			}
		}
		cancel()
	}
	if ctx.Err() != nil {
		// Keep the assignment for recovery when the node is stopping.
		return ctx.Err()
	}
	result.Answer = strings.TrimSpace(result.Answer)
	for _, secret := range append(outputSecrets(job.Lease), providerKey) {
		if secret == "" {
			continue
		}
		result.Answer = strings.ReplaceAll(result.Answer, secret, "[已隐藏]")
		if result.Action != nil {
			result.Action.Title = strings.ReplaceAll(result.Action.Title, secret, "[已隐藏]")
			result.Action.Body = strings.ReplaceAll(result.Action.Body, secret, "[已隐藏]")
		}
	}
	if turnErr != nil || result.Answer == "" || len(result.Answer) > 16<<10 || strings.ContainsRune(result.Answer, 0) {
		result.fail()
		if providerFailure != "" {
			result.Error = providerFailure
		}
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

// Intermediate snapshots are best effort; only the regular result receipt commits a turn.
func (w *worker) chatPreview(ctx context.Context, job managementChatJob, secrets []string) func(string) {
	var mu sync.Mutex
	var previous string
	var retryAfter time.Time
	return func(answer string) {
		mu.Lock()
		defer mu.Unlock()
		if ctx.Err() != nil || time.Now().Before(retryAfter) || len(answer) > 16<<10 || strings.ContainsRune(answer, 0) {
			return
		}
		answer = redactChatPreview(answer, secrets)
		if answer == previous {
			return
		}
		call, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		status, err := w.post(call, "/internal/management-chat/progress", map[string]any{"id": job.ID, "lease": job.Lease, "answer": answer}, nil)
		if err != nil || status != 204 {
			retryAfter = time.Now().Add(2 * time.Second)
			return
		}
		previous = answer
	}
}

func redactChatPreview(answer string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			answer = strings.ReplaceAll(answer, secret, "[已隐藏]")
		}
	}
	// A key split across chunks must not become visible one prefix at a time.
	trim := 0
	for _, secret := range secrets {
		for n := min(len(answer), len(secret)-1); n > trim; n-- {
			if strings.HasSuffix(answer, secret[:n]) {
				trim = n
				break
			}
		}
	}
	return answer[:len(answer)-trim]
}
