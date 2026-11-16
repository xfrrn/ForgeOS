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

func fixture(t *testing.T) (*worker, store.Job, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := &worker{repoRoot: filepath.Join(base, "repos"), workspaceRoot: filepath.Join(base, "workspaces")}
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

func TestRetryReportWithoutExecutingTwice(t *testing.T) {
	w, job, head := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	w.token = "test-worker"
	var claims, reports int
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+w.token || r.Method != http.MethodPost {
			t.Error("invalid request")
			rw.WriteHeader(401)
			return
		}
		switch r.URL.Path {
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
