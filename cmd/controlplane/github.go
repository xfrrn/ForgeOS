package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
	"forgeos/schemas"
)

var (
	githubOrigin     = regexp.MustCompile(`(?i)^(?:https://github\.com/|ssh://git@github\.com(?::22)?/|git@github\.com:)([a-z0-9][a-z0-9_-]*)/([a-z0-9_.-]+)/?$`)
	gitCommit        = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	errMissing       = errors.New("repository or issue not found on GitHub, or unavailable to this account")
	errGHRead        = errors.New("无法读取 GitHub，请检查网络和仓库访问权限。")
	errGHLimit       = errors.New("GitHub rate limit reached")
	errIdentity      = errors.New("GitHub response does not match the registered origin and requested issue")
	errNotOpen       = errors.New("select an open issue; closed issues and pull requests cannot be dispatched")
	errIssueText     = errors.New("issue title or body is invalid or exceeds the supported size")
	errOrigin        = errors.New("registered project requires an accessible Git working tree with a github.com HTTPS or SSH origin")
	errBase          = errors.New("project requires a committed local HEAD")
	errProjectConfig = errors.New("invalid project.yaml at the selected base commit")
)

type issueStore interface {
	Project(context.Context, int64) (store.Project, error)
	ImportIssue(context.Context, int64, store.IssueSpec, ...store.FeedbackSource) (store.Task, error)
}

type issueImporter interface {
	issueStore
	FeedbackDispatch(context.Context, int64, int64, store.FeedbackSource) (*store.Task, error)
}

type githubGET func(context.Context, string, any) error

func registerIssueRoute(mux *http.ServeMux, data *store.Store, root, owner string) {
	mux.HandleFunc("POST /tasks/{id}/requirements/{revision}/apply", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, n, ok := requirementPath(w, r)
		if !ok {
			return
		}
		task, err := data.AdoptRequirement(r.Context(), id, n)
		respond(w, http.StatusOK, task, err)
	}))
	mux.HandleFunc("GET /tasks/{id}/requirements/{revision}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, n, ok := requirementPath(w, r)
		if !ok {
			return
		}
		rev, err := data.RequirementRevision(r.Context(), id, n)
		respond(w, http.StatusOK, rev, err)
	}))
	mux.HandleFunc("GET /tasks/{id}/comments", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var after int64
		if raw := r.URL.Query().Get("after"); raw != "" {
			var err error
			after, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || after < 0 {
				problem(w, http.StatusBadRequest, "provide a non-negative event cursor")
				return
			}
		}
		events, err := data.Comments(r.Context(), id, after)
		respond(w, http.StatusOK, events, err)
	}))
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
	mux.HandleFunc("GET /projects/{id}/issues/{number}", authorized(owner, viewGitHubIssue(data, root, ghRead)))
	mux.HandleFunc("GET /projects/{id}/classifications/{classification}/processes", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("classification"), 10, 64)
		if err != nil || id < 1 {
			problem(w, 400, "invalid issue classification")
			return
		}
		history, err := data.IssueClassificationProcesses(r.Context(), project, id)
		respond(w, http.StatusOK, history, err)
	}))
}

func viewGitHubIssue(data issueStore, root string, get githubGET) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
		if err != nil || number <= 0 {
			problem(w, http.StatusBadRequest, "请输入有效的 Issue 编号。")
			return
		}
		ctx := r.Context()
		project, err := data.Project(ctx, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		_, issue, err := readGitHubIssue(ctx, root, project, get, number)
		if err != nil {
			githubIssueProblem(w, err)
			return
		}
		respond(w, http.StatusOK, struct {
			store.Issue
			SourceDigest string `json:"source_digest"`
		}{issue, issueSourceDigest(issue)}, nil)
	}
}

func readGitHubIssue(ctx context.Context, root string, project store.Project, get githubGET, number int64) (githubProject, store.Issue, error) {
	repo, err := githubSource(ctx, root, project, get)
	if err != nil {
		return repo, store.Issue{}, err
	}
	var raw githubIssue
	if err := get(ctx, fmt.Sprintf("repositories/%d/issues/%d", repo.ID, number), &raw); err != nil {
		return repo, store.Issue{}, err
	}
	issue, err := repo.issue(raw, number)
	return repo, issue, err
}

func githubIssueProblem(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	case errors.Is(err, errMissing):
		status = http.StatusNotFound
	case errors.Is(err, errGHLimit):
		status = http.StatusTooManyRequests
	case errors.Is(err, errIdentity), errors.Is(err, errNotOpen), errors.Is(err, errIssueText), errors.Is(err, errOrigin), errors.Is(err, errBase), errors.Is(err, errProjectConfig):
		status = http.StatusConflict
	}
	problem(w, status, err.Error())
}

func importIssue(data issueImporter, root string, get githubGET) http.HandlerFunc {
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
			Validation      string                `json:"validation"`
			Mode            string                `json:"mode"`
			Harness         string                `json:"harness"`
			TaskType        string                `json:"task_type"`
			WorkType        string                `json:"work_type"`
			RequireWorkType bool                  `json:"require_work_type"`
			SourceDigest    string                `json:"source_digest"`
			Target          string                `json:"target"`
			Feedback        *store.FeedbackSource `json:"feedback_source"`
		}
		if !decode(w, r, &input) {
			return
		}
		policy := validation.Policy{TaskType: input.TaskType, Target: input.Target}
		if input.WorkType != "" && !store.ValidWorkType(input.WorkType) {
			problem(w, http.StatusBadRequest, "unsupported Issue work type")
			return
		}
		if !(store.IssueSpec{Harness: input.Harness, Mode: input.Mode}).ValidHarness() {
			problem(w, http.StatusBadRequest, "select codex or claude for implementation, or codex for planning")
			return
		}
		if !store.ValidValidation(input.Validation) || !policy.Valid() {
			problem(w, http.StatusBadRequest, "select auto, docs, go-test or V1 through V4 and a supported task type and delivery target")
			return
		}
		if input.Mode != "" && input.Mode != "plan" {
			problem(w, http.StatusBadRequest, "unsupported issue execution mode")
			return
		}
		ctx := r.Context()
		var sources []store.FeedbackSource
		if input.Feedback != nil {
			prior, err := data.FeedbackDispatch(ctx, id, number, *input.Feedback)
			if err != nil || prior != nil {
				respond(w, http.StatusOK, prior, err)
				return
			}
			sources = append(sources, *input.Feedback)
		}
		project, err := data.Project(ctx, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		source, issue, err := readGitHubIssue(ctx, root, project, get, number)
		if err != nil {
			githubIssueProblem(w, err)
			return
		}
		if input.SourceDigest != "" && input.SourceDigest != issueSourceDigest(issue) {
			problem(w, http.StatusConflict, "the GitHub Issue changed; refresh the inbox before dispatch")
			return
		}
		if input.WorkType != "" {
			if store.ValidWorkType(issue.WorkType) && input.WorkType != issue.WorkType {
				problem(w, http.StatusConflict, "work type differs from the current GitHub Issue")
				return
			}
			issue.OwnerClassified = !store.ValidWorkType(issue.WorkType)
			if issue.OwnerClassified && issue.WorkType == "clarification" {
				issue.ClassificationDigest = issueSourceDigest(issue)
			}
			issue.WorkType = input.WorkType
		}
		if issue.WorkType == "clarification" || (input.RequireWorkType && !store.ValidWorkType(issue.WorkType)) {
			problem(w, http.StatusConflict, "select an Issue work type before dispatch")
			return
		}
		spec := source.spec(issue, input.Validation, input.Mode, policy)
		spec.ImportOrigin = "owner"
		spec.ExplicitHarness = input.Harness != ""
		if input.Harness != "" && spec.Mode == "plan" && input.Mode != "plan" {
			spec.ChildHarness = input.Harness
		} else if input.Harness != "" {
			spec.Harness = input.Harness
		}
		task, err := data.ImportIssue(ctx, id, spec, sources...)
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
	if _, err := exec.LookPath("gh"); err != nil {
		return publicGitHubRead(ctx, path, dest)
	}
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

func githubHTTPClient(ctx context.Context) (*http.Client, error) {
	proxy, err := cloneProxy(ctx, os.TempDir(), "https://github.com/")
	if err != nil {
		return nil, errGHRead
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != "" {
		parsed, err := url.Parse(proxy)
		if err != nil {
			return nil, errGHRead
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func publicGitHubRead(ctx context.Context, path string, dest any) error {
	target, err := url.Parse("https://api.github.com/" + strings.TrimPrefix(path, "/"))
	if err != nil || target.Host != "api.github.com" || target.Scheme != "https" {
		return errGHRead
	}
	client, err := githubHTTPClient(ctx)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return errGHRead
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ForgeOS")
	// Use the service account's existing GitHub credential only for this host.
	if token := githubGitCredential(ctx); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errGHRead
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return errMissing
	case res.StatusCode == http.StatusTooManyRequests || (res.StatusCode == http.StatusForbidden && res.Header.Get("X-RateLimit-Remaining") == "0"):
		return errGHLimit
	case res.StatusCode != http.StatusOK:
		return errGHRead
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 12<<20)).Decode(dest); err != nil {
		return errGHRead
	}
	return nil
}

func githubGitCredential(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := localrepo.GitCommand(ctx, os.TempDir(), "-c", "credential.helper=", "-c", "credential.helper=manager", "credential", "fill")
	cmd.Env = append(cmd.Env, "GCM_INTERACTIVE=Never")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	data, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if token, ok := strings.CutPrefix(strings.TrimSuffix(line, "\r"), "password="); ok {
			return token
		}
	}
	return ""
}

type githubProject struct {
	ID         int64  `json:"id"`
	FullName   string `json:"full_name"`
	Base       string `json:"-"`
	Declared   bool   `json:"-"`
	Harness    string `json:"-"`
	ConfigBlob string `json:"-"`
}

func (repo githubProject) spec(issue store.Issue, profile, mode string, policy validation.Policy) store.IssueSpec {
	if store.ValidWorkType(issue.WorkType) && issue.WorkType != "development" {
		mode = "plan"
	}
	spec := store.IssueSpec{Issue: issue, BaseCommit: repo.Base, Validation: profile, Mode: mode}
	spec.ProjectConfigBlob = repo.ConfigBlob
	if mode == "plan" {
		spec.ChildHarness = repo.Harness
	} else {
		spec.Harness = repo.Harness
	}
	if repo.Declared || profile == "auto" || policy != (validation.Policy{}) {
		switch profile {
		case "docs":
			policy.Minimum = "V1"
		case "go-test":
			policy.Minimum = "V2"
		default:
			if validation.ValidLevel(profile) {
				policy.Minimum = profile
			}
		}
		if policy.Target == "" || policy.Target == "local" {
			policy.Target = "pull_request"
		}
		spec.Validation, spec.Policy = "auto", &policy
	}
	return spec
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
	config, err := localrepo.Git(ctx, path, "ls-tree", "--name-only", repo.Base, "--", "acceptance.yaml", "validation.yaml")
	if err != nil {
		return repo, errBase
	}
	repo.Declared = len(config) != 0
	blob, raw, err := validation.ReadConfigFile(ctx, path, repo.Base, "project.yaml")
	if err != nil {
		return repo, errProjectConfig
	}
	if blob != "" {
		repo.Harness, err = schemas.ProjectHarness(raw)
		if err != nil {
			return repo, errProjectConfig
		}
		repo.ConfigBlob = blob
	}
	if repo.ID <= 0 || !strings.EqualFold(repo.FullName, slug) || (project.RepositoryID != 0 && repo.ID != project.RepositoryID) {
		return repo, errIdentity
	}
	return repo, nil
}

type githubIssue struct {
	Comments    int             `json:"comments"`
	ID          int64           `json:"id"`
	Number      int64           `json:"number"`
	RepoURL     string          `json:"repository_url"`
	HTMLURL     string          `json:"html_url"`
	State       string          `json:"state"`
	ClosedAt    string          `json:"closed_at"`
	StateReason string          `json:"state_reason"`
	PullRequest json.RawMessage `json:"pull_request"`
	Title, Body string
	UpdatedAt   string                  `json:"updated_at"`
	Labels      []struct{ Name string } `json:"labels"`
}

func (repo githubProject) matchesIssue(remote githubIssue, number int64) bool {
	return number > 0 && remote.Number == number && remote.ID > 0 && remote.RepoURL == "https://api.github.com/repos/"+repo.FullName &&
		remote.HTMLURL == fmt.Sprintf("https://github.com/%s/issues/%d", repo.FullName, number)
}

func (repo githubProject) issue(remote githubIssue, number int64) (store.Issue, error) {
	if !repo.matchesIssue(remote, number) {
		return store.Issue{}, errIdentity
	}
	if remote.State != "open" || remote.PullRequest != nil {
		return store.Issue{}, errNotOpen
	}
	if _, err := time.Parse(time.RFC3339, remote.UpdatedAt); err != nil {
		return store.Issue{}, errIdentity
	}
	if strings.TrimSpace(remote.Title) == "" || len(remote.Title) > 2048 || len(remote.Body) > 256<<10 ||
		strings.ContainsRune(remote.Title, 0) || strings.ContainsRune(remote.Body, 0) {
		return store.Issue{}, errIssueText
	}
	kind, labels := remote.classify()
	return store.Issue{WorkType: kind, WorkTypeLabels: labels, RepositoryID: repo.ID, Repository: repo.FullName, ID: remote.ID, Number: number,
		URL: remote.HTMLURL, Title: remote.Title, Body: remote.Body, UpdatedAt: remote.UpdatedAt}, nil
}

func requirementPath(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return 0, 0, false
	}
	n, err := strconv.ParseInt(r.PathValue("revision"), 10, 32)
	if err != nil || n < 1 {
		problem(w, http.StatusBadRequest, "provide a positive requirement revision")
		return 0, 0, false
	}
	return id, n, true
}
