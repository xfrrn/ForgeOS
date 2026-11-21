package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/validation"
)

type chatIssueDraft struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	WorkType string `json:"work_type"`
}

func validChatIssue(title, body, kind string) bool {
	return title != "" && utf8.RuneCountInString(title) <= 256 && strings.IndexFunc(title, unicode.IsControl) < 0 && body != "" && len(body) <= 16<<10 && !strings.ContainsRune(body, 0) && store.ValidWorkType(kind)
}

type chatIssueClient func(context.Context, store.Project) (deliveryAPI, error)
type issueAPIStatus int

var errIssueNotSent = errors.New("GitHub connection was not established")

func (e issueAPIStatus) Error() string { return fmt.Sprintf("GitHub HTTP %d", e) }

func (c projectCloneService) chatIssueAPI(ctx context.Context, p store.Project) (deliveryAPI, error) {
	var token, scope string
	if p.RepositoryCredentialID != 0 {
		path, err := localrepo.ResolveRepo(ctx, c.localRoot, p.Path)
		if err != nil {
			return nil, err
		}
		origin, err := localrepo.Git(ctx, path, "remote", "get-url", "origin")
		if err != nil {
			return nil, err
		}
		slug := repositoryName(strings.TrimSpace(string(origin)))
		if slug == "" {
			return nil, errOrigin
		}
		remote, err := parseCloneRemote("https://github.com/"+slug, "")
		if err != nil {
			return nil, err
		}
		if err = c.authenticateRemote(ctx, p.RepositoryCredentialID, &remote); err != nil {
			return nil, err
		}
		token = remote.credential.token
		scope = "repos/" + strings.ToLower(slug)
	} else {
		token = os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}
		if token == "" {
			if exe, err := exec.LookPath("gh"); err == nil {
				call, cancel := context.WithTimeout(ctx, 3*time.Second)
				output, err := exec.CommandContext(call, exe, "auth", "token", "--hostname", "github.com").Output()
				cancel()
				if err == nil {
					token = strings.TrimSpace(string(output))
				}
			}
		}
		if token == "" {
			token = githubGitCredential(ctx)
		}
	}
	client, err := githubHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, method, endpoint string, body, dest any) error {
		defer client.CloseIdleConnections()
		path, _, _ := strings.Cut(strings.ToLower(endpoint), "?")
		if scope != "" && path != scope && !strings.HasPrefix(path, scope+"/") {
			return errOrigin
		}
		if method == http.MethodPost && token == "" {
			return issueAPIStatus(401)
		}
		var input io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				return err
			}
			input = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com/"+endpoint, input)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "ForgeOS")
		if input != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		var connected atomic.Bool
		if method == http.MethodPost {
			trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }}
			req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
		}
		resp, err := client.Do(req)
		if err != nil {
			if method == http.MethodPost && !connected.Load() {
				return errIssueNotSent
			}
			return err
		}
		defer resp.Body.Close()
		if (method == http.MethodPost && resp.StatusCode != 201) || (method == http.MethodGet && resp.StatusCode != 200) {
			return issueAPIStatus(resp.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(dest)
	}, nil
}

func (c projectChat) issueRepository(ctx context.Context, project int64) (githubProject, deliveryAPI, error) {
	var repo githubProject
	p, err := c.data.Project(ctx, project)
	if err != nil {
		return repo, nil, err
	}
	if c.issueClient == nil {
		return repo, nil, errOrigin
	}
	api, err := c.issueClient(ctx, p)
	if err != nil {
		return repo, nil, err
	}
	repo, err = githubSource(ctx, c.root, p, func(ctx context.Context, path string, out any) error { return api(ctx, "GET", path, nil, out) })
	return repo, api, err
}

// GitHub Issues does not offer a create idempotency key. An invisible per-card
// marker lets us recover an acknowledged or timed-out write without posting again.
func findChatIssue(ctx context.Context, api deliveryAPI, path, marker string, started time.Time) (githubIssue, error) {
	since := url.QueryEscape(started.UTC().Add(-24 * time.Hour).Format(time.RFC3339))
	for page := 1; ; page++ {
		var batch []githubIssue
		err := api(ctx, "GET", fmt.Sprintf("%s?state=all&since=%s&per_page=100&page=%d", path, since, page), nil, &batch)
		if err != nil {
			return githubIssue{}, err
		}
		for _, issue := range batch {
			if issue.PullRequest == nil && strings.HasSuffix(strings.ReplaceAll(issue.Body, "\r\n", "\n"), marker) {
				return issue, nil
			}
		}
		if len(batch) < 100 {
			return githubIssue{}, nil
		}
	}
}

func (c projectChat) confirmNewIssue(w http.ResponseWriter, r *http.Request, project int64, a store.ChatAction, draft chatIssueDraft) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(65 * time.Second))
	repo, api, err := c.issueRepository(r.Context(), project)
	if err != nil {
		problem(w, 502, "无法读取目标 GitHub 仓库，请检查网络和仓库凭据。")
		return
	}
	if repo.ID != a.RepositoryID || !strings.EqualFold(repo.FullName, a.Repository) {
		problem(w, 409, "项目仓库已变更，请重新生成草稿。")
		return
	}
	title, body, kind := a.Label, a.Body, a.WorkType
	first := false
	if a.CreationStarted == nil {
		if draft.Title != "" {
			title = strings.TrimSpace(draft.Title)
		}
		if draft.Body != "" {
			body = strings.ReplaceAll(strings.TrimSpace(draft.Body), "\r\n", "\n")
		}
		if draft.WorkType != "" {
			kind = draft.WorkType
		}
		if !validChatIssue(title, body, kind) {
			problem(w, 400, "请填写有效的标题、Issue 正文和工作类型。")
			return
		}
		if classified, _ := (githubIssue{Title: title}).classify(); classified != "" && classified != kind {
			problem(w, 400, "标题中的工作类型与所选类型不一致。")
			return
		}
		first, err = c.data.BeginChatIssue(r.Context(), project, a.ID, title, body, kind)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
	}
	// Finish the already authorized write even if the browser navigates away.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 35*time.Second)
	defer cancel()
	a, err = c.data.ChatAction(ctx, project, a.ID)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	if a.Status != "pending" {
		turn, err := c.data.ChatTurn(ctx, project, a.TurnID)
		respond(w, 200, turn, err)
		return
	}
	if a.CreationStarted == nil {
		problem(w, 409, "上次创建未获接受，请重新确认。")
		return
	}
	marker := "\n\n<!-- forgeos-chat-issue:" + a.CreateKey + " -->"
	path := "repos/" + repo.FullName + "/issues"
	var raw githubIssue
	switch {
	case a.IssueNumber > 0:
		err = api(ctx, "GET", fmt.Sprintf("%s/%d", path, a.IssueNumber), nil, &raw)
	case first:
		err = api(ctx, "POST", path, map[string]string{"title": a.Label, "body": a.Body + marker}, &raw)
		var code issueAPIStatus
		if errors.Is(err, errIssueNotSent) || (errors.As(err, &code) && (code == 400 || code == 401 || code == 403 || code == 404 || code == 410 || code == 422 || code == 429)) {
			if resetErr := c.data.RejectChatIssue(ctx, project, a.ID); resetErr != nil {
				respond(w, 0, nil, resetErr)
				return
			}
			if errors.Is(err, errIssueNotSent) {
				problem(w, 502, "未能连接 GitHub，请检查网络后重新确认。")
			} else {
				problem(w, 502, fmt.Sprintf("GitHub 拒绝创建 Issue（HTTP %d）。请核对仓库是否启用 Issues、Issues 写入权限及请求限制后重试。", code))
			}
			return
		}
	default:
		raw, err = findChatIssue(ctx, api, path, marker, *a.CreationStarted)
	}
	raw.Body = strings.ReplaceAll(raw.Body, "\r\n", "\n")
	if err != nil || !repo.matchesIssue(raw, raw.Number) || (a.IssueNumber == 0 && !strings.HasSuffix(raw.Body, marker)) {
		problem(w, 502, "GitHub 创建结果尚未核实。稍后点击此卡继续核对，系统会查找同一次创建记录。")
		return
	}
	if err = c.data.RecordChatIssue(ctx, project, a.ID, raw.Number); err != nil {
		respond(w, 0, nil, err)
		return
	}
	var spec *store.IssueSpec
	issue, err := repo.issue(raw, raw.Number)
	if err == nil && issue.Title == a.Label && issue.Body == a.Body+marker && (issue.WorkType == "" || issue.WorkType == a.WorkType) {
		issue.OwnerClassified = issue.WorkType == ""
		issue.WorkType = a.WorkType
		frozen := repo.spec(issue, "auto", "", validation.Policy{})
		frozen.ImportOrigin = "owner"
		spec = &frozen
	} else if err != nil && !errors.Is(err, errNotOpen) {
		problem(w, 502, "Issue 已创建，暂时无法验证派发信息。请在此卡继续。")
		return
	}
	turn, err := c.data.ConfirmChatAction(ctx, project, a.ID, spec)
	if err != nil {
		problem(w, 502, "Issue 已创建，派发未完成。请在此卡继续派发。")
		return
	}
	respond(w, 200, turn, nil)
}
