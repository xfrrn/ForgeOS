package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func TestRepositoryName(t *testing.T) {
	for origin, want := range map[string]string{
		"https://github.com/org/.github.git":        "org/.github",
		"git@github.com:org/project.git":            "org/project",
		"ssh://git@github.com:22/org/project.git":   "org/project",
		"https://token@github.com/org/project.git":  "",
		"https://github.com.example.org/org/repo":   "",
		"https://github.com/org/../issues":          "",
		"https://github.com/org/..":                 "",
		"https://github.com/org/project?thing=evil": "",
		"file:///tmp/git-mirror":                    "",
	} {
		if got := repositoryName(origin); got != want {
			t.Errorf("repositoryName(%q) = %q, want %q", origin, got, want)
		}
	}
}

type importFixture struct {
	path string
	spec *store.IssueSpec
}

func (f *importFixture) Project(context.Context, int64) (store.Project, error) {
	return store.Project{ID: 3, Path: f.path}, nil
}

func (f *importFixture) FeedbackDispatch(context.Context, int64, int64, store.FeedbackSource) (*store.Task, error) {
	return nil, nil
}

func (f *importFixture) ImportIssue(_ context.Context, _ int64, spec store.IssueSpec, _ ...store.FeedbackSource) (store.Task, error) {
	f.spec = &spec
	return store.Task{ID: 12, Spec: &spec, Kind: "codex_issue"}, nil
}

func issueTestRepo(t *testing.T) (root, project, base string) {
	t.Helper()
	root = t.TempDir()
	project = filepath.Join(root, "repo")
	ctx := context.Background()
	if _, err := localrepo.Git(ctx, root, "init", "--quiet", project); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"remote", "add", "origin", "git@github.com:my-org/project.git"},
		{"-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "--allow-empty", "-qm", "base"},
	} {
		if _, err := localrepo.Git(ctx, project, args...); err != nil {
			t.Fatal(err)
		}
	}
	head, err := localrepo.Git(ctx, project, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return root, project, strings.TrimSpace(string(head))
}

func TestImportSelectedGitHubIssue(t *testing.T) {
	root, project, base := issueTestRepo(t)
	for _, tc := range []struct {
		name   string
		status int
	}{
		{"selected", 200}, {"wrong-role", 401}, {"no-profile", 400}, {"remote-mismatch", 409},
		{"issue-mismatch", 409}, {"pull-request", 409}, {"closed", 409}, {"missing", 404}, {"timeout", 504},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := &importFixture{path: project}
			calls := 0
			get := func(_ context.Context, path string, dst any) error {
				calls++
				switch path {
				case "repos/my-org/project":
					slug := "my-org/project"
					if tc.name == "remote-mismatch" {
						slug = "other-org/project"
					}
					b, _ := json.Marshal(map[string]any{"id": 88, "full_name": slug})
					return json.Unmarshal(b, dst)
				case "repositories/88/issues/7":
					switch tc.name {
					case "timeout":
						return context.DeadlineExceeded
					case "missing":
						return errMissing
					}
					issue := map[string]any{"id": 367, "number": 7, "title": "Fix the README", "body": "Edit one example",
						"state": "open", "updated_at": "2026-01-05T16:00:00Z", "html_url": "https://github.com/my-org/project/issues/7",
						"repository_url": "https://api.github.com/repos/my-org/project"}
					switch tc.name {
					case "issue-mismatch":
						issue["number"] = 8
					case "pull-request":
						issue["pull_request"] = map[string]string{}
					case "closed":
						issue["state"] = "closed"
					}
					b, _ := json.Marshal(issue)
					return json.Unmarshal(b, dst)
				default:
					t.Fatalf("unexpected GitHub endpoint %s", path)
					return nil
				}
			}
			mux := http.NewServeMux()
			mux.HandleFunc("POST /projects/{id}/issues/{number}", authorized("owner-token", importIssue(data, root, get)))
			body, token := `{"validation":"docs"}`, "owner-token"
			if tc.name == "no-profile" {
				body = `{}`
			}
			if tc.name == "wrong-role" {
				token = "worker-token"
			}
			req := httptest.NewRequest("POST", "/projects/3/issues/7", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			res := httptest.NewRecorder()
			mux.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", res.Code, tc.status, res.Body.String())
			}
			if tc.name != "selected" {
				if data.spec != nil || ((tc.status == 401 || tc.status == 400) && calls != 0) {
					t.Fatal("invalid request reached an unauthorized operation")
				}
				return
			}
			s := data.spec
			if calls != 2 || s == nil || s.ImportOrigin != "owner" || s.Issue.ID != 367 || s.Issue.RepositoryID != 88 ||
				s.Issue.Body != "Edit one example" || s.Validation != "docs" || s.BaseCommit != base {
				t.Fatalf("missing task evidence: %+v", s)
			}
		})
	}
}
