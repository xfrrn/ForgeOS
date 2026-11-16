package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestDeliveryConfig(t *testing.T) {
	for _, tc := range []struct {
		id, target string
		valid      bool
	}{
		{"", "", true}, {"", "main", false}, {"0", "", false}, {"1", "", true},
		{"2", "release/v1", true}, {"2", "HEAD", false}, {"2", "bad..branch", false},
	} {
		_, err := deliveryConfig(tc.id, tc.target)
		if (err == nil) != tc.valid {
			t.Errorf("deliveryConfig(%q, %q): %v", tc.id, tc.target, err)
		}
	}
}

func TestAcceptanceToDelivery(t *testing.T) {
	db := testdb.Open(t)
	data := store.New(db)
	ctx := context.Background()
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root, repo, base := issueTestRepo(t)
	project, err := data.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	policy := deliveryPolicy{projectID: project.ID, target: "main"}
	mux := routes(data, root, "owner", "worker", policy)
	post := func(path, token string, body any, status int) {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s: HTTP %d, expected %d: %s", path, w.Code, status, w.Body.String())
		}
	}
	spec := store.IssueSpec{Issue: store.Issue{ID: 15, RepositoryID: 10, Repository: "my-org/project", Number: 2, Title: "Fix README"}, BaseCommit: base, Validation: "docs"}
	task, err := data.ImportIssue(ctx, project.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	job, err := data.Claim(ctx, "worker", t.TempDir(), true)
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		out, err := localrepo.Git(ctx, dir, args...)
		if err != nil {
			t.Fatalf("git %s: %v", args[0], err)
		}
		return strings.TrimSpace(string(out))
	}
	git(repo, "worktree", "add", "--detach", job.Workspace, base)
	if err := os.WriteFile(filepath.Join(job.Workspace, "README.md"), []byte("verified\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git(job.Workspace, "add", "README.md")
	git(job.Workspace, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "Fix README")
	candidate := git(job.Workspace, "rev-parse", "HEAD")
	post("/internal/executions/"+strconv.FormatInt(job.ExecutionID, 10)+"/result", "worker", map[string]any{
		"lease_token": job.LeaseToken, "exit_code": 0, "session_id": "test-session", "base_commit": base, "commit": candidate,
		"validation": map[string]any{"profile": "docs", "commit": candidate, "exit_code": 0, "output": "pass"},
	}, 204)
	path := "/tasks/" + strconv.FormatInt(task.ID, 10) + "/acceptance"
	body := map[string]any{"execution_id": job.ExecutionID, "commit": base, "decision": "approve"}
	post(path, "owner", body, 409)
	body["commit"] = candidate
	post(path, "worker", body, 401)
	post(path, "owner", body, 200)
	post(path, "owner", body, 200)

	// Only the GitHub boundary is simulated here. The acceptance, queue and audit use real PostgreSQL.
	states := []string{"WAITING", "MERGED", "DONE"}
	mergeSHA := strings.Repeat("b", 40)
	for _, state := range states {
		claimed, err := policy.step(ctx, store.New(db), root, func(_ context.Context, dir string, frozen store.Task, d store.Delivery) (store.Delivery, error) {
			if dir != repo || frozen.Execution.Result.Commit != candidate || d.Commit != candidate || d.ExecutionID != job.ExecutionID {
				t.Fatal("lost frozen delivery identity")
			}
			d.State = state
			switch state {
			case "WAITING":
				d.PRNumber, d.PRURL = 42, "https://github.com/my-org/project/pull/42"
			case "MERGED":
				d.MergeSHA = mergeSHA
			}
			return d, nil
		})
		if err != nil || !claimed {
			t.Fatalf("deliver %s: claimed=%t err=%v", state, claimed, err)
		}
	}
	final, err := store.New(db).Task(ctx, task.ID)
	if err != nil || final.Status != "DONE" || final.Delivery.MergeSHA != mergeSHA || len(final.Reviews) != 1 {
		t.Fatalf("final: %+v, %v", final, err)
	}
	post(path, "owner", body, 200)
	extra, err := data.ClaimDelivery(ctx, project.ID)
	if err != nil || extra != nil {
		t.Fatalf("completed delivery claimed again: %v", err)
	}
}
