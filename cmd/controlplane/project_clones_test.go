package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestCloneRemote(t *testing.T) {
	for input, expected := range map[string]string{
		"https://github.com/Octocat/Hello-World":      "https://github.com/octocat/hello-world.git",
		"git@github.com:Octocat/Hello-World.git":      "https://github.com/octocat/hello-world.git",
		"ssh://git@github.com:22/Octocat/Hello-World": "https://github.com/octocat/hello-world.git",
		"https://gitlab.com:443/workgroup/team/code":  "https://gitlab.com/workgroup/team/code.git",
		"https://bitbucket.org/workgroup/team":        "https://bitbucket.org/workgroup/team.git",
		"https://git.example.net/workgroup/team":      "https://git.example.net/workgroup/team.git",
	} {
		remote, err := parseCloneRemote(input, "git.example.net")
		if err != nil || remote.URL != expected {
			t.Fatalf("%q -> %q: %v", input, remote.URL, err)
		}
	}
	for _, input := range []string{
		"https://another.example.net/org/repo", "http://github.com/org/repo", "file:///org/repo",
		"https://user:password@github.com/org/repo", "https://github.com/org/repo#main",
		"https://gitlab.com/org/repo/-/tree/main", "https://github.com/org/repo?x=1",
		"https://github.com/org/repo/tree/main", "https://github.com:10443/org/repo", "https://github.com:/org/repo",
		"https://gitlab.com/group/../repo", "https://gitlab.com/group/%2F/repo", "ssh://root@github.com/org/repo",
		"https://gitlab.com/group/.git/repo", "https://.../group/repo",
	} {
		if _, err := parseCloneRemote(input, "..."); err == nil {
			t.Fatalf("accepted invalid remote %q", input)
		}
	}
}

func TestProjectCloneRoutes(t *testing.T) {
	ctx := context.Background()
	data := store.New(testdb.Open(t))
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	local, registered, _ := issueTestRepo(t)
	managed := filepath.Join(t.TempDir(), "repositories")
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", managed)
	handler := routes(data, local, "owner", legacyWorker("worker"), nil)
	registerProjectChat(handler, data, local, "owner", nil, ghRead)
	registerProjectWorkViews(handler, data, "owner")
	service := projectCloneService{data: data, localRoot: local}
	service.register(handler, "owner")
	request := func(method, path, token, body string, status int) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: %d != %d: %s", method, path, w.Code, status, w.Body)
		}
		return w.Body.String()
	}
	request(http.MethodGet, "/projects/clone-settings", "worker", "", 401)
	request(http.MethodPut, "/projects/clone-settings", "owner", `{"directory":"../escape"}`, 400)
	request(http.MethodPut, "/projects/clone-settings", "owner", `{"directory":"team-a"}`, 200)
	request(http.MethodPost, "/projects/clone", "owner", `{"url":"https://github.com/Octocat/Hello-World"}`, 202)
	request(http.MethodPost, "/projects/clone", "owner", `{"url":"git@github.com:octocat/hello-world.git"}`, 202)
	job, err := data.ClaimRepositoryClone(ctx)
	if err != nil || job == nil || filepath.Base(job.Directory) != "team-a" || job.Repository != "octocat/hello-world" {
		t.Fatalf("missing queued clone job or selected directory: %+v %v", job, err)
	}
	request(http.MethodPut, "/projects/clone-settings", "owner", `{"directory":"team-b"}`, 200)
	if os.Getenv("FORGEOS_TEST_GIT_CLONE") == "1" {
		service.execute(ctx, *job)
	} else {
		// The default test exercises registration and the Worker boundary offline.
		parent, err := localrepo.ResolveManagedDirectory(filepath.Join(job.Directory, "github.com", "octocat"))
		if err != nil {
			t.Fatal(err)
		}
		checkout := filepath.Join(parent, "hello-world")
		if _, err := localrepo.Git(ctx, parent, "init", "--quiet", checkout); err != nil {
			t.Fatal(err)
		}
		if err := data.CompleteRepositoryClone(ctx, job.ID, job.Lease, checkout); err != nil {
			t.Fatal(err)
		}
	}
	finished, err := data.RepositoryClone(ctx, job.ID)
	if err != nil || finished.ProjectID == 0 || finished.Status != "done" {
		t.Fatalf("repository was not registered: %+v %v", finished, err)
	}
	checkout := finished.Path
	rel, err := filepath.Rel(job.Directory, checkout)
	if err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("clone was not placed in the selected directory: %q", checkout)
	}
	if os.Getenv("FORGEOS_TEST_GIT_CLONE") == "1" {
		if _, err := localrepo.Git(ctx, checkout, "rev-parse", "--verify", "HEAD"); err != nil {
			t.Fatalf("remote clone did not contain a committed checkout: %v", err)
		}
	}
	wire := localrepo.RelativePath(local, checkout)
	if got, err := localrepo.ResolveAssigned(ctx, local, checkout, wire); err != nil || got != checkout {
		t.Fatalf("Worker cannot resolve managed checkout: %q via %q: %v", got, wire, err)
	}
	endpoint := fmt.Sprintf("/projects/clone/jobs/%d", job.ID)
	if read := request(http.MethodGet, endpoint, "owner", "", 200); !strings.Contains(read, `"status":"done"`) ||
		!strings.Contains(read, `"repository":"octocat/hello-world"`) {
		t.Fatal("clone receipt lacks a project and its repository name")
	}
	request(http.MethodPost, "/projects/clone", "owner", `{"url":"https://github.com/octocat/hello-world.git"}`, 200)
	request(http.MethodPost, "/projects", "owner", fmt.Sprintf(`{"path":%q}`, filepath.Base(registered)), 200)
}
