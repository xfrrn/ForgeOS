package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func TestRecoveredQuestionOnlySettlesAfterClaimingAnswer(t *testing.T) {
	for _, mode := range []string{"shutdown", "claimed_elsewhere", "version_changed", "resource_sequence"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w, j, base := fixture(t)
			j.Kind = "codex_issue"
			j.LeaseToken = strings.Repeat("a", 64)
			j.LeaseSeconds = 300
			j.ResumeQuestions = true
			j.ManageResources = mode == "resource_sequence"
			j.Spec = &store.IssueSpec{BaseCommit: base, Validation: "docs"}
			w.codex = "unusable-test-binary"
			w.codexVersion = "current"
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			reported := make(chan store.Result, 1)
			polled := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/internal/executions/12/checkpoint-question":
					var q store.CheckpointQuestion
					if err := json.NewDecoder(r.Body).Decode(&q); err != nil || len(q.ResumeInstance) != 64 || q.Session != "original" {
						t.Error("invalid resume request", err)
					}
					polled <- struct{}{}
					switch mode {
					case "shutdown":
						cancel()
					case "claimed_elsewhere":
						rw.WriteHeader(http.StatusPreconditionFailed)
					case "version_changed", "resource_sequence":
						json.NewEncoder(rw).Encode(map[string]string{"answer": "Use Harbor"})
					}
				case "/internal/executions/12/renew":
					rw.WriteHeader(http.StatusNoContent)
				case "/internal/executions/12/resource":
					var request store.ResourceRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Step != 2 || request.Kind != "model" {
						t.Error("reused resource step after restart", request.Step, err)
					}
					rw.WriteHeader(http.StatusConflict)
				case "/internal/executions/12/result":
					var result store.Result
					if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
						t.Error(err)
					}
					reported <- result
					rw.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					rw.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			w.api, w.client = server.URL, server.Client()
			db, err := w.openJournal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			w.journal = db
			defer db.Close()
			key := sha256.Sum256([]byte(j.LeaseToken))
			if _, err := db.Exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace) VALUES(?,?,?,?)", key[:], j.ExecutionID, j.TaskID, j.Workspace); err != nil {
				t.Fatal(err)
			}
			dir, err := checkedWorktree(ctx, j.RepoPath, j.Workspace, base)
			if err != nil {
				t.Fatal(err)
			}
			trees, err := checkpointTrees(ctx, j.Workspace)
			if err != nil {
				t.Fatal(err)
			}
			q := parkedQuestion{Job: j, Question: "Which title?", Evidence: "No name selected", Version: "previous", Session: "original", GitDir: dir, Trees: trees, Remaining: time.Minute}
			if j.ManageResources {
				q.Version = w.codexVersion
				q.ResourceStep = 1
			}
			if err = w.parkQuestion(ctx, q); err != nil {
				t.Fatal(err)
			}
			saved, err := w.readParkedQuestion(ctx, j.ExecutionID)
			if err != nil || saved == nil {
				t.Fatal(err)
			}
			if err = w.runAttempt(ctx, j, time.Now(), saved); err != nil {
				t.Fatal(err)
			}
			if len(polled) != 1 {
				t.Fatal("did not check original question")
			}
			ids, err := w.parkedExecutions(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if mode == "version_changed" || mode == "resource_sequence" {
				if len(ids) != 0 || len(reported) != 1 {
					t.Fatal("missing terminal receipt", ids)
				}
				want := "version changed"
				if j.ManageResources {
					want = "model resource request rejected"
				}
				if r := <-reported; r.ExitCode == 0 || !strings.Contains(r.Error, want) || r.Commit != "" {
					t.Fatal(r)
				}
			} else if len(ids) != 1 || len(reported) != 0 {
				t.Fatal("settled a checkpoint without owning the resumed turn", ids)
			}
		})
	}
}
