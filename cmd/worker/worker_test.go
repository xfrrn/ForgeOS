package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := localrepo.Git(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func TestChildClaimWithLargeDecisionContext(t *testing.T) {
	// A single plan can reference enough 4 KiB statements to cross the role-job response limit.
	rows := make([]map[string]any, 520)
	for i := range rows {
		rows[i] = map[string]any{"id": i + 1, "statement": strings.Repeat("x", 4096)}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"goal_context": map[string]any{"decisions": rows}})
	}))
	defer server.Close()
	w := worker{api: server.URL, client: server.Client(), token: "worker"}
	var job store.Job
	if _, err := w.post(context.Background(), "/internal/claim", struct{}{}, &job); err != nil || len(job.GoalContext) <= 2<<20 {
		t.Fatal(len(job.GoalContext), err)
	}
	var roleJob store.CurationJob
	if _, err := w.post(context.Background(), "/internal/curator/claim", struct{}{}, &roleJob); err == nil {
		t.Fatal("unexpectedly accepted oversized Curator response")
	}
}

func fixture(t *testing.T) (*worker, store.Job, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := &worker{repoRoot: filepath.Join(base, "repos"), workspaceRoot: filepath.Join(base, "workspaces"), slots: 1}
	repo := filepath.Join(w.repoRoot, "project")
	for _, dir := range []string{repo, w.workspaceRoot} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "init", "--quiet")
	git(t, repo, "-c", "user.name=Worker Test", "-c", "user.email=test@localhost", "commit", "--quiet", "--allow-empty", "-m", "initial")
	job := store.Job{ExecutionID: 12, TaskID: 7, RepoPath: repo, Workspace: filepath.Join(w.workspaceRoot, "execution-12"), LeaseToken: "test-lease"}
	return w, job, git(t, repo, "rev-parse", "HEAD")
}

func TestGitHeadInIndependentWorktree(t *testing.T) {
	w, job, head := fixture(t)
	// An inherited GIT_DIR must not redirect a Worker to an unrelated repository.
	t.Setenv("GIT_DIR", filepath.Join(w.repoRoot, "missing"))
	got := w.execute(context.Background(), job)
	if got != (store.Result{Commit: head}) {
		t.Fatalf("unexpected result: %+v", got)
	}
	if git(t, job.Workspace, "rev-parse", "HEAD") != head {
		t.Fatal("wrong workspace revision")
	}
	if _, err := localrepo.Git(context.Background(), job.Workspace, "symbolic-ref", "-q", "HEAD"); err == nil {
		t.Fatal("expected detached HEAD")
	}
	if got := w.execute(context.Background(), job); got.ExitCode == 0 {
		t.Fatal("reused an existing execution workspace")
	}
}

func TestRejectsUnregisteredRepositoryAndWorkspacePaths(t *testing.T) {
	w, job, _ := fixture(t)
	ctx := context.Background()
	subdir := filepath.Join(job.RepoPath, "nested")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{subdir, "..", w.workspaceRoot} {
		if _, err := localrepo.ResolveRepo(ctx, w.repoRoot, path); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	// A linked checkout can live inside an allowed directory while its Git storage lives outside.
	link := filepath.Join(w.workspaceRoot, "linked")
	git(t, job.RepoPath, "worktree", "add", "--detach", "--no-checkout", link, "HEAD")
	if _, err := localrepo.ResolveRepo(ctx, w.workspaceRoot, link); err == nil {
		t.Fatal("accepted out-of-scope Git storage")
	}
	if path, err := localrepo.ResolveRepo(ctx, w.repoRoot, "project"); err != nil || path != job.RepoPath {
		t.Fatalf("relative path: %s, %v", path, err)
	}
	job.Workspace = filepath.Join(job.RepoPath, "execution-12")
	if got := w.execute(ctx, job); got.ExitCode == 0 {
		t.Fatal("accepted external execution path")
	}
}

func TestAssignedProjectAtNodeMount(t *testing.T) {
	w, job, head := fixture(t)
	root := t.TempDir()
	registered := filepath.Join(root, "project")
	job.RepoRelativePath = localrepo.RelativePath(root, registered)
	job.RepoPath = registered
	if job.RepoRelativePath != "project" {
		t.Fatal("missing portable project path")
	}
	result := w.execute(context.Background(), job)
	if result.ExitCode != 0 || result.Commit != head {
		t.Fatalf("node mount: %+v", result)
	}
	if _, err := localrepo.ResolveAssigned(context.Background(), w.repoRoot, registered, "../project"); err == nil {
		t.Fatal("accepted a path outside the node root")
	}
	// A broader node root may already contain the registered absolute checkout.
	path, err := localrepo.ResolveAssigned(context.Background(), filepath.Dir(w.repoRoot), filepath.Join(w.repoRoot, "project"), "project")
	if err != nil || path != filepath.Join(w.repoRoot, "project") {
		t.Fatal("lost existing absolute project path", path, err)
	}
}

func TestRetryReportWithoutExecutingTwice(t *testing.T) {
	w, job, head := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	w.token = "test-worker"
	var claims, reports int
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+w.token || r.Method != http.MethodPost {
			t.Error("invalid request")
			rw.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/internal/workers/ci-cleanup":
			rw.Write([]byte("[]"))
		case "/internal/workers/heartbeat":
			rw.WriteHeader(204)
		case "/internal/claim":
			claims++
			if claims == 1 {
				_ = json.NewEncoder(rw).Encode(job)
				return
			}
			rw.WriteHeader(204)
			cancel()
		case fmt.Sprintf("/internal/executions/%d/result", job.ExecutionID):
			var got struct {
				LeaseToken string `json:"lease_token"`
				store.Result
			}
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.Result != (store.Result{Commit: head}) || got.LeaseToken != job.LeaseToken {
				t.Errorf("invalid result: %+v, %v", got, err)
			}
			reports++
			if reports == 1 {
				rw.WriteHeader(503)
				return
			}
			rw.WriteHeader(204)
		default:
			t.Error("unexpected path")
			rw.WriteHeader(404)
		}
	}))
	defer s.Close()
	w.api, w.client = s.URL, s.Client()
	if err := w.run(ctx); err != nil {
		t.Fatal(err)
	}
	if claims != 2 || reports != 2 {
		t.Fatalf("claims=%d reports=%d", claims, reports)
	}
}

func TestReplayDurableReceipt(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ctx := context.Background()
			w, job, head := fixture(t)
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			var first json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				var report json.RawMessage
				if r.URL.Path != "/internal/executions/12/result" || json.NewDecoder(r.Body).Decode(&report) != nil {
					t.Error("unexpected report request")
				}
				if first == nil {
					first = report
					rw.WriteHeader(http.StatusForbidden)
					return
				}
				if string(first) != string(report) {
					t.Error("replayed receipt differs from the original")
				}
				rw.WriteHeader(status)
			}))
			defer server.Close()
			w.api, w.client, w.id = server.URL, server.Client(), "node"
			db, err := w.openJournal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			w.journal = db
			err = w.executeAndReport(ctx, job, time.Now())
			db.Close()
			if err == nil {
				t.Fatal("unconfirmed result was reported as confirmed")
			}
			w.journal, err = w.openJournal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			if err := w.replayReceipts(ctx); err != nil {
				t.Fatal(err)
			}
			var outcome, candidate, conflict string
			var cleared bool
			err = w.journal.QueryRow("SELECT outcome,candidate,report IS NULL,coalesce(conflicting_result,'') FROM attempts").Scan(&outcome, &candidate, &cleared, &conflict)
			if err != nil || candidate != head || !cleared || strings.Contains(conflict, job.LeaseToken) {
				t.Fatal("invalid recovery record", err)
			}
			if status == http.StatusConflict && (outcome != "conflict" || !strings.Contains(conflict, head)) ||
				status == http.StatusNoContent && (outcome != "confirmed" || conflict != "") {
				t.Fatalf("wrong outcome %s", outcome)
			}
		})
	}
}

func TestLeaseRenewalStopsAtAuthorityBoundary(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprintf("unavailable=%t", unavailable), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				requests++
				var body map[string]string
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["lease_token"] != "lease" || r.URL.Path != "/internal/executions/1/renew" {
					t.Error("wrong lease identity")
				}
				if unavailable || requests == 1 {
					rw.WriteHeader(http.StatusServiceUnavailable)
				} else if requests == 2 {
					rw.WriteHeader(http.StatusNoContent)
				} else {
					rw.WriteHeader(http.StatusConflict)
				}
			}))
			defer server.Close()
			w := worker{client: server.Client(), api: server.URL}
			ttl := 300 * time.Millisecond
			err := w.renewWhileRunning(ctx, store.Job{ExecutionID: 1, LeaseToken: "lease"}, ttl, time.Now().Add(ttl))
			if err == nil || ctx.Err() != nil {
				t.Fatal("lease loss went undetected", err)
			}
			if unavailable && !strings.Contains(err.Error(), "expired") || !unavailable && !strings.Contains(err.Error(), "409") {
				t.Fatal(err)
			}
		})
	}
}
