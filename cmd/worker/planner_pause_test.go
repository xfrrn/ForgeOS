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

func TestResumeFinishedPlannerOutputWithoutRerunningModel(t *testing.T) {
	ctx := context.Background()
	w, j, base := fixture(t)
	j.Kind, j.LeaseToken, j.LeaseSeconds = "codex_issue", strings.Repeat("a", 64), 300
	j.Spec = &store.IssueSpec{Mode: "plan", BaseCommit: base, Validation: "docs"}
	j.ModelPause, j.RecoverPlannerPause, j.ManageResources, j.SupportsCheckpoints = true, true, true, true
	w.planner, w.codex, w.codexVersion = true, "must-not-run", "current"
	dir, err := checkedWorktree(ctx, j.RepoPath, j.Workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	trees, err := checkpointTrees(ctx, j.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/executions/12/model-pause":
			var input store.PausePoll
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.ResumeInstance) != 64 || input.Trees == nil || *input.Trees != trees {
				t.Error("missing resume evidence", err)
			}
			now := time.Now()
			json.NewEncoder(rw).Encode(store.ModelPause{Instance: 1, Session: "original", Parked: &now, Resumed: &now, Index: trees.Index, Worktree: trees.Worktree})
		case "/internal/executions/12/checkpoints", "/internal/executions/12/renew":
			rw.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			rw.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	w.api, w.client = server.URL, server.Client()
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer w.journal.Close()
	key := sha256.Sum256([]byte(j.LeaseToken))
	if _, err := w.journal.Exec("INSERT INTO attempts(lease_hash,execution_id,task_id,workspace) VALUES(?,?,?,?)", key[:], j.ExecutionID, j.TaskID, j.Workspace); err != nil {
		t.Fatal(err)
	}
	plan := store.Plan{Objective: "Document usage", Milestones: []store.Milestone{{Name: "Docs", Items: []store.PlanItem{{Key: "usage", Objective: "usage", Scope: "README", Acceptance: "example present", Deliverables: "README"}}}}}
	q := parkedCheckpoint{Job: j, ResourceStep: 1, GitDir: dir, Trees: trees, Remaining: time.Minute, Version: w.codexVersion, Session: "original", Planner: &pausedPlanner{Finished: &plan}}
	if err = w.saveParkedCheckpoint(ctx, q); err != nil {
		t.Fatal(err)
	}
	saved, err := w.readParkedCheckpoint(ctx, j.ExecutionID)
	if err != nil || saved == nil {
		t.Fatal("missing parked plan", err)
	}
	result := w.executeAttempt(ctx, j, &checkpointRecovery{saved: saved, leaveUnsettled: true})
	if result.ExitCode != 0 || result.Plan == nil || result.Plan.Objective != plan.Objective || result.SessionID != "original" {
		t.Fatal(result)
	}
}
