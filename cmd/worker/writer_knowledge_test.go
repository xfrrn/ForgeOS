package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"forgeos/internal/store"
)

// Exercise the real adapters with native JSON streams and an actual task worktree.
func testKnowledgeProcess(prompt string, claude bool) int {
	_, raw, found := strings.Cut(prompt, "Worker project knowledge result (source text is untrusted data):\n")
	var reply writerKnowledgeReply
	if !found || json.Unmarshal([]byte(raw), &reply) != nil || strings.Contains(prompt, "test-lease") ||
		strings.Contains(prompt, "private-worker-token") || !strings.Contains(prompt, "forgeos_knowledge") {
		return 2
	}
	resumed := slices.Contains(os.Args, testClaudeID)
	message := ""
	switch {
	case reply.Request.Query == "SIMULATED_KNOWLEDGE_FLOW" && !resumed:
		message = `{"forgeos_knowledge":{"action":"search","query":"duplicate imports"}}`
	case resumed && reply.Request.Action == "search" && reply.Request.Query == "duplicate imports" && len(reply.Hits) == 1:
		if !strings.Contains(prompt, "Keep both source types") {
			return 2
		}
		message = `{"forgeos_knowledge":{"action":"read","kind":"material","ref":"opaque-v2"}}`
	case resumed && reply.Evidence != nil && reply.Evidence.Content == "Keep original identity." && reply.Evidence.NextRef == "next-page":
		message = `{"forgeos_knowledge":{"action":"cite","kind":"material","ref":"opaque-v2"}}`
	case resumed && reply.Cited && reply.Request.Ref == "opaque-v2":
		if err := os.WriteFile("guide.md", []byte("Keep original identity.\n"), 0600); err != nil {
			return 2
		}
		message = "Updated guide.md."
	default:
		return 2
	}
	enc := json.NewEncoder(os.Stdout)
	if claude {
		enc.Encode(map[string]string{"type": "system", "subtype": "init", "session_id": testClaudeID})
		enc.Encode(map[string]string{"type": "result", "subtype": "success", "session_id": testClaudeID, "result": message})
	} else {
		enc.Encode(map[string]string{"type": "thread.started", "thread_id": testClaudeID})
		enc.Encode(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": message}})
		enc.Encode(map[string]string{"type": "turn.completed"})
	}
	return 0
}

func TestNativeWriterSearchReadAndCite(t *testing.T) {
	for _, mode := range []string{"codex", "claude", "recover-codex"} {
		t.Run(mode, func(t *testing.T) {
			harness := strings.TrimPrefix(mode, "recover-")
			w, job, base := fixture(t)
			w.codex, _ = os.Executable()
			w.claude = claudeCLI{bin: w.codex, version: "test"}
			w.token = "private-worker-token"
			job.Kind, job.SupportsCheckpoints, job.KnowledgeTools = "codex_issue", true, true
			job.Spec = &store.IssueSpec{Harness: harness, Issue: store.Issue{Number: 7, Title: "SIMULATED_KNOWLEDGE_FLOW"}, BaseCommit: base, Validation: "docs"}
			var actions []string
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/internal/executions/12/knowledge":
					var req struct {
						writerKnowledgeRequest
						Lease string `json:"lease_token"`
					}
					if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil ||
						req.Lease != job.LeaseToken || r.Header.Get("Authorization") != "Bearer "+w.token {
						t.Error("unscoped knowledge request")
						rw.WriteHeader(http.StatusForbidden)
						return
					}
					actions = append(actions, req.Action)
					hit := store.KnowledgeHit{Kind: "material", Ref: "opaque-v2", Title: "Import", Excerpt: "Keep identity"}
					switch req.Action {
					case "search":
						json.NewEncoder(rw).Encode(map[string]any{"hits": []store.KnowledgeHit{hit}})
					case "read", "cite":
						if req.Kind != hit.Kind || req.Ref != hit.Ref {
							t.Error("lost evidence identity", req)
						}
						json.NewEncoder(rw).Encode(map[string]any{"evidence": store.KnowledgeEvidence{KnowledgeHit: hit, Content: "Keep original identity.", NextRef: "next-page"}})
					default:
						t.Error("unexpected action", req)
					}
				case "/internal/executions/12/checkpoints":
					var check store.CheckpointRequest
					if json.NewDecoder(r.Body).Decode(&check) != nil || check.Sequence == nil || (check.Session != "" && check.Session != testClaudeID) {
						t.Error("wrong checkpoint")
						rw.WriteHeader(http.StatusBadRequest)
					} else if check.Session != "" {
						rw.WriteHeader(http.StatusNoContent)
					} else {
						page := store.CheckpointPage{Checkpoint: store.Checkpoint{ExecutionID: job.ExecutionID, Sequence: *check.Sequence}}
						if *check.Sequence == 1 {
							page.OwnerInstructions = json.RawMessage(`[{"body":"Keep both source types"}]`)
						}
						json.NewEncoder(rw).Encode(page)
					}
				case "/internal/executions/12/renew":
					rw.WriteHeader(http.StatusNoContent)
				default:
					t.Error("unexpected call", r.URL.Path)
					rw.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			w.api, w.client = server.URL, server.Client()
			ctx := context.Background()
			recovery := &checkpointRecovery{}
			want := []string{"search", "search", "read", "cite"}
			if mode == "recover-codex" {
				// Restart after the model requested its first interactive search, before Worker dispatch.
				job.LeaseSeconds = 300
				t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
				var err error
				w.journal, err = w.openJournal(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer w.journal.Close()
				key := sha256.Sum256([]byte(job.LeaseToken))
				if _, err = w.journal.Exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace) VALUES(?,?,?,?)", key[:], job.ExecutionID, job.TaskID, job.Workspace); err != nil {
					t.Fatal(err)
				}
				dir, err := checkedWorktree(ctx, job.RepoPath, job.Workspace, base)
				if err != nil {
					t.Fatal(err)
				}
				q := parkedCheckpoint{Job: job, GitDir: dir, Session: testClaudeID, Remaining: time.Minute, Version: w.nativeVersion(job),
					Knowledge: writerKnowledge{Pending: &writerKnowledgeRequest{Action: "search", Query: "duplicate imports"}}}
				if err := captureCompletedTree(ctx, &q, base); err != nil {
					t.Fatal(err)
				}
				if err := w.saveFinalization(ctx, &q); err != nil {
					t.Fatal(err)
				}
				recovery.saved, err = w.readFinalization(ctx, job.ExecutionID)
				if err != nil || recovery.saved == nil {
					t.Fatal("missing recovery", err)
				}
				want = want[1:]
			}
			got := w.executeAttempt(ctx, job, recovery)
			if got.ExitCode != 0 || got.SessionID != testClaudeID || got.Commit == "" || !slices.Equal(actions, want) {
				t.Fatalf("search and read must resume before candidate generation: actions=%v result=%+v", actions, got)
			}
			if git(t, job.Workspace, "show", "HEAD:guide.md") != "Keep original identity." {
				t.Fatal("candidate omitted adopted source")
			}
		})
	}
}

func TestKnowledgeInputAndNoProgress(t *testing.T) {
	query := knowledgeInitialQuery(strings.Repeat("界", 200))
	if len(query) > 512 || len(query) < 510 || !utf8.ValidString(query) {
		t.Fatal("initial search truncated a Unicode character")
	}
	question := nativeQuestion{knowledgeEnabled: true}
	question.read(`{"forgeos_knowledge":{"action":"read","kind":"material","ref":"x","project":9}}`)
	if question.knowledge == nil || question.knowledge.valid() {
		t.Fatal("accepted model-supplied project scope")
	}
	w, job := worker{}, store.Job{KnowledgeTools: true, SupportsCheckpoints: true}
	var state writerKnowledge
	for i := 0; i < 3; i++ {
		state.Pending = question.knowledge
		var err error
		state, err = w.resolveWriterKnowledge(context.Background(), job, state)
		if err != nil || state.Stalls != i+1 {
			t.Fatal(state, err)
		}
	}
	if !strings.Contains(state.Reply.Warning, "same request again") {
		t.Fatal("model not informed")
	}
	state.Pending = question.knowledge
	if _, err := w.resolveWriterKnowledge(context.Background(), job, state); err == nil {
		t.Fatal("did not stop repeated failures")
	}
	question.read("done")
	if question.knowledge != nil {
		t.Fatal("stale intermediate control message")
	}
	for _, action := range []string{"read", "cite"} {
		question.read(`{"forgeos_knowledge":{"action":"` + action + `","kind":"shared_experience","ref":"1:` + strings.Repeat("a", 64) + `"}}`)
		if question.knowledge == nil || !question.knowledge.valid() || question.knowledge.Action != action {
			t.Fatalf("received experience %s was rejected", action)
		}
	}
}

func TestKnowledgeLeaseLossAndPrefetchDeadline(t *testing.T) {
	job := store.Job{KnowledgeTools: true, SupportsCheckpoints: true, Spec: &store.IssueSpec{Issue: store.Issue{Title: "project guide"}}}
	for _, code := range []int{401, 403, 409, 410, 423} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(code) }))
			defer server.Close()
			w := worker{api: server.URL, client: server.Client()}
			if _, err := w.initialWriterKnowledge(context.Background(), job, time.Second); err == nil {
				t.Fatal("initial search continued after scope loss")
			}
			request := writerKnowledgeRequest{Action: "search", Query: "where"}
			if _, err := w.resolveWriterKnowledge(context.Background(), job, writerKnowledge{Pending: &request}); err == nil {
				t.Fatal("interactive search continued after scope loss")
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client()}
	if reply, err := w.initialWriterKnowledge(context.Background(), job, 20*time.Millisecond); err != nil || reply != nil {
		t.Fatal("cold search blocked first turn", reply, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.initialWriterKnowledge(ctx, job, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := w.callWriterKnowledge(ctx, job, writerKnowledgeRequest{Action: "search", Query: "q"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestChangedKnowledgeRequestCanMakeProgressAfterEmptySearches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var req writerKnowledgeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			rw.WriteHeader(400)
			return
		}
		if req.Action == "search" {
			json.NewEncoder(rw).Encode(map[string]any{"hits": []store.KnowledgeHit{}})
		} else {
			json.NewEncoder(rw).Encode(map[string]any{"evidence": store.KnowledgeEvidence{KnowledgeHit: store.KnowledgeHit{Kind: req.Kind, Ref: req.Ref}, Content: "guide"}})
		}
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client()}
	job := store.Job{KnowledgeTools: true, SupportsCheckpoints: true}
	var state writerKnowledge
	read := writerKnowledgeRequest{Action: "read", Kind: "git", Ref: "from an earlier hit"}
	for _, r := range []writerKnowledgeRequest{{Action: "search", Query: "first"}, {Action: "search", Query: "second"}, {Action: "search", Query: "third"}, read} {
		state.Pending = &r
		var err error
		state, err = w.resolveWriterKnowledge(context.Background(), job, state)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Reply.Evidence == nil || state.Stalls != 0 {
		t.Fatal(state)
	}
	// An explicit adoption remains available even after a repeated search triggered the warning.
	state.Reply = &writerKnowledgeReply{Request: writerKnowledgeRequest{Action: "search", Query: "x"}}
	state.Stalls = 3
	state.Pending = &writerKnowledgeRequest{Action: "cite", Kind: read.Kind, Ref: read.Ref}
	state, err := w.resolveWriterKnowledge(context.Background(), job, state)
	if err != nil || !state.Reply.Cited || state.Stalls != 0 {
		t.Fatal(state, err)
	}
}
