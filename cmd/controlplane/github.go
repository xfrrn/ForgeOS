package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

var (
	githubOrigin = regexp.MustCompile(`(?i)^(?:https://github\.com/|ssh://git@github\.com(?::22)?/|git@github\.com:)([a-z0-9][a-z0-9_-]*)/([a-z0-9_.-]+)/?$`)
	gitCommit    = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	errMissing   = errors.New("repository or issue not found on GitHub, or unavailable to this account")
	errGHRead    = errors.New("GitHub read failed; check gh availability, authentication and repository access")
	errGHLimit   = errors.New("GitHub rate limit reached")
	errIdentity  = errors.New("GitHub response does not match the registered origin and requested issue")
	errNotOpen   = errors.New("select an open issue; closed issues and pull requests cannot be dispatched")
	errIssueText = errors.New("issue title or body is invalid or exceeds the supported size")
	errOrigin    = errors.New("registered project requires an accessible Git working tree with a github.com HTTPS or SSH origin")
	errBase      = errors.New("project requires a committed local HEAD")
)

type issueStore interface {
	Project(context.Context, int64) (store.Project, error)
	ImportIssue(context.Context, int64, store.IssueSpec) (store.Task, error)
}

type githubGET func(context.Context, string, any) error

func registerIssueRoute(mux *http.ServeMux, data *store.Store, root, owner string) {
	mux.HandleFunc("GET /projects/{id}/inbox", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var before int64
		if q := r.URL.Query(); q.Has("before") {
			var err error
			before, err = strconv.ParseInt(q.Get("before"), 10, 64)
			if err != nil || before < 1 {
				problem(w, http.StatusBadRequest, "before must be a positive issue number")
				return
			}
		}
		items, err := data.Inbox(r.Context(), id, before)
		respond(w, http.StatusOK, items, err)
	}))
	mux.HandleFunc("POST /projects/{id}/issues/{number}", authorized(owner, importIssue(data, root, ghRead)))
}

func importIssue(data issueStore, root string, get githubGET) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, valid := pathID(w, r)
		if !valid {
			return
		}
		number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
		if err != nil || number <= 0 {
			problem(w, http.StatusBadRequest, "invalid issue number")
			return
		}
		var input struct {
			Validation string `json:"validation"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Validation != "docs" && input.Validation != "go-test" {
			problem(w, http.StatusBadRequest, "select a validation profile: docs or go-test")
			return
		}
		ctx := r.Context()
		project, err := data.Project(ctx, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		source, err := githubSource(ctx, root, project, get)
		var issue store.Issue
		if err == nil {
			var remote githubIssue
			err = get(ctx, fmt.Sprintf("repositories/%d/issues/%d", source.ID, number), &remote)
			if err == nil {
				issue, err = source.issue(remote, number)
			}
		}
		if err != nil {
			status := http.StatusBadGateway
			switch {
			case errors.Is(err, context.DeadlineExceeded):
				status = http.StatusGatewayTimeout
			case errors.Is(err, errMissing):
				status = http.StatusNotFound
			case errors.Is(err, errGHLimit):
				status = http.StatusTooManyRequests
			case errors.Is(err, errIdentity), errors.Is(err, errNotOpen), errors.Is(err, errIssueText), errors.Is(err, errOrigin), errors.Is(err, errBase):
				status = http.StatusConflict
			}
			problem(w, status, err.Error())
			return
		}
		task, err := data.ImportIssue(ctx, id, store.IssueSpec{Issue: issue, BaseCommit: source.Base, Validation: input.Validation})
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "GitHub issue or repository conflicts with the stored project mapping")
			return
		}
		respond(w, http.StatusOK, task, err)
	}
}

func repositoryName(origin string) string {
	parts := githubOrigin.FindStringSubmatch(origin)
	if parts == nil {
		return ""
	}
	name := strings.TrimSuffix(parts[2], ".git")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return parts[1] + "/" + name
}

func ghRead(ctx context.Context, path string, dest any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "api", "--hostname", "github.com", "--method", "GET", path).Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			switch {
			case bytes.Contains(exit.Stderr, []byte("(HTTP 404)")):
				return errMissing
			case bytes.Contains(exit.Stderr, []byte("(HTTP 429)")), bytes.Contains(bytes.ToLower(exit.Stderr), []byte("rate limit")):
				return errGHLimit
			}
		}
		return errGHRead
	}
	if err := json.Unmarshal(out, dest); err != nil {
		return errGHRead
	}
	return nil
}

type githubProject struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Base     string `json:"-"`
}

func githubSource(ctx context.Context, root string, project store.Project, get githubGET) (githubProject, error) {
	var repo githubProject
	path, err := localrepo.ResolveRepo(ctx, root, project.Path)
	if err != nil {
		return repo, errOrigin
	}
	origin, err := localrepo.Git(ctx, path, "remote", "get-url", "origin")
	slug := repositoryName(strings.TrimSpace(string(origin)))
	if err != nil || slug == "" {
		return repo, errOrigin
	}
	head, err := localrepo.Git(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	repo.Base = strings.TrimSpace(string(head))
	if err != nil || !gitCommit.MatchString(repo.Base) {
		return repo, errBase
	}
	if err := get(ctx, "repos/"+slug, &repo); err != nil {
		return repo, err
	}
	if repo.ID <= 0 || !strings.EqualFold(repo.FullName, slug) || (project.RepositoryID != 0 && repo.ID != project.RepositoryID) {
		return repo, errIdentity
	}
	return repo, nil
}

type githubIssue struct {
	ID          int64           `json:"id"`
	Number      int64           `json:"number"`
	RepoURL     string          `json:"repository_url"`
	HTMLURL     string          `json:"html_url"`
	State       string          `json:"state"`
	PullRequest json.RawMessage `json:"pull_request"`
	Title, Body string
	UpdatedAt   string                  `json:"updated_at"`
	Labels      []struct{ Name string } `json:"labels"`
}

func (repo githubProject) issue(remote githubIssue, number int64) (store.Issue, error) {
	htmlURL := fmt.Sprintf("https://github.com/%s/issues/%d", repo.FullName, number)
	if number <= 0 || remote.Number != number || remote.ID <= 0 || remote.RepoURL != "https://api.github.com/repos/"+repo.FullName {
		return store.Issue{}, errIdentity
	}
	if remote.State != "open" || remote.PullRequest != nil {
		return store.Issue{}, errNotOpen
	}
	if remote.HTMLURL != htmlURL {
		return store.Issue{}, errIdentity
	}
	if _, err := time.Parse(time.RFC3339, remote.UpdatedAt); err != nil {
		return store.Issue{}, errIdentity
	}
	if strings.TrimSpace(remote.Title) == "" || len(remote.Title) > 2048 || len(remote.Body) > 256<<10 ||
		strings.ContainsRune(remote.Title, 0) || strings.ContainsRune(remote.Body, 0) {
		return store.Issue{}, errIssueText
	}
	return store.Issue{RepositoryID: repo.ID, Repository: repo.FullName, ID: remote.ID, Number: number,
		URL: htmlURL, Title: remote.Title, Body: remote.Body, UpdatedAt: remote.UpdatedAt}, nil
}
