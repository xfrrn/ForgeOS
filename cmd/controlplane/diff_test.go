package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

type diffFixture struct {
	task    store.Task
	project store.Project
}

func (f *diffFixture) Task(_ context.Context, id int64) (store.Task, error) {
	if id != f.task.ID {
		return store.Task{}, store.ErrNotFound
	}
	return f.task, nil
}

func (f *diffFixture) Project(_ context.Context, id int64) (store.Project, error) {
	if id != f.project.ID {
		return store.Project{}, store.ErrNotFound
	}
	return f.project, nil
}

func TestTaskDiffIsBoundToRecordedAttempt(t *testing.T) {
	root, repo, base := issueTestRepo(t)
	git := func(args ...string) string {
		t.Helper()
		b, err := localrepo.Git(context.Background(), repo, args...)
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(b))
	}
	for name, content := range map[string]string{"README.md": "recorded first line\n" + strings.Repeat("more text\n", maxDiffBytes/8), ".gitattributes": "*.md diff=custom\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "change")
	tip := git("rev-parse", "HEAD")
	git("config", "diff.custom.textconv", "nonexistent-textconv")
	git("config", "diff.external", "nonexistent-diff-tool")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("uncommitted change"), 0600); err != nil {
		t.Fatal(err)
	}
	f := diffFixture{
		task: store.Task{ID: 1, ProjectID: 2, Kind: "codex_issue", Spec: &store.IssueSpec{BaseCommit: base, Issue: store.Issue{RepositoryID: 3}},
			Attempts: []store.Execution{{ID: 4, Result: &store.Result{BaseCommit: base, Commit: tip}}, {ID: 5}}},
		project: store.Project{ID: 2, RepositoryID: 3, Path: repo},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /tasks/{id}/diff", authorized("owner", taskDiff(&f, root)))
	get := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	valid := "/tasks/1/diff?execution_id=4&commit=" + tip
	w := get(valid, "owner")
	var result commitDiff
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.Truncated || len(result.Diff) != maxDiffBytes ||
		!strings.Contains(result.Diff, "+recorded first line") || strings.Contains(result.Diff, "uncommitted change") {
		t.Fatalf("bad history preview: HTTP %d, bytes=%d, truncated=%v", w.Code, len(result.Diff), result.Truncated)
	}
	for _, tc := range []struct {
		path, token string
		code        int
	}{
		{valid, "worker", 401},
		{"/tasks/1/diff?execution_id=4&commit=HEAD", "owner", 400},
		{"/tasks/1/diff?execution_id=4&commit=" + base, "owner", 409},
		{"/tasks/1/diff?execution_id=5&commit=" + tip, "owner", 409},
		{"/tasks/1/diff?execution_id=6&commit=" + tip, "owner", 404},
		{"/tasks/9/diff?execution_id=4&commit=" + tip, "owner", 404},
	} {
		if w := get(tc.path, tc.token); w.Code != tc.code {
			t.Errorf("%s returned %d, want %d", tc.path, w.Code, tc.code)
		}
	}
	f.project.Path = t.TempDir()
	if w := get(valid, "owner"); w.Code != 409 {
		t.Fatalf("repository outside root returned %d", w.Code)
	}
}
