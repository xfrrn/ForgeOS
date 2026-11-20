package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"forgeos/internal/agents"
	"forgeos/internal/store"
	"forgeos/internal/validation"
	"github.com/google/uuid"
)

type chatNodes map[int64][sha256.Size]byte

func loadChatNodes(file, owner, manager string, workers *workerRegistry) (chatNodes, error) {
	nodes := make(chatNodes)
	if file == "" {
		return nodes, nil
	}
	if !filepath.IsAbs(file) {
		return nil, errors.New("FORGEOS_MANAGEMENT_CHAT_NODES_FILE must be an absolute path")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.New("cannot read FORGEOS_MANAGEMENT_CHAT_NODES_FILE")
	}
	var configs []struct {
		Project int64  `json:"project_id"`
		Digest  string `json:"token_sha256"`
	}
	if json.Unmarshal(raw, &configs) != nil || len(configs) == 0 {
		return nil, errors.New("configure a nonempty array of Management chat node grants")
	}
	used := make(map[[sha256.Size]byte]bool)
	for _, token := range []string{"", owner, manager, os.Getenv("FORGEOS_CURATOR_TOKEN"),
		os.Getenv("FORGEOS_REVIEWER_TOKEN"), os.Getenv("FORGEOS_TEST_AGENT_TOKEN")} {
		used[sha256.Sum256([]byte(token))] = true
	}
	for _, grant := range workers.snapshot() {
		used[grant.digest] = true
	}
	for _, config := range configs {
		raw, err := hex.DecodeString(config.Digest)
		if err != nil || len(raw) != sha256.Size || config.Project < 1 {
			return nil, errors.New("chat node grants require a positive project_id and a SHA-256 digest")
		}
		digest := [sha256.Size]byte(raw)
		_, exists := nodes[config.Project]
		if used[digest] || exists {
			return nil, errors.New("each Management chat project needs a distinct role credential")
		}
		used[digest] = true
		nodes[config.Project] = digest
	}
	workers.mu.Lock()
	defer workers.mu.Unlock()
	if workers.reserved == nil {
		workers.reserved = make(map[[sha256.Size]byte]bool)
	}
	for _, digest := range nodes {
		workers.reserved[digest] = true
	}
	return nodes, nil
}

func (n chatNodes) authorize(next func(int64, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		hash := sha256.Sum256([]byte(token))
		var project int64
		for id, digest := range n {
			if subtle.ConstantTimeCompare(digest[:], hash[:]) == 1 {
				project = id
			}
		}
		if !bearer || token == "" || project == 0 {
			problem(w, http.StatusUnauthorized, "invalid Management chat credentials")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		next(project, w, r.WithContext(ctx))
	}
}

type projectChat struct {
	data *store.Store
	root string
	get  githubGET
}

func registerProjectChat(mux *http.ServeMux, data *store.Store, root, owner string, nodes chatNodes, get githubGET) {
	chat := projectChat{data: data, root: root, get: get}
	mux.HandleFunc("GET /projects/{id}/chat", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		var before int64
		if r.URL.Query().Has("before") {
			var err error
			before, err = strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
			if err != nil || before < 1 {
				problem(w, http.StatusBadRequest, "无效的聊天翻页位置。")
				return
			}
		}
		ctx := r.Context()
		if _, err := data.Project(ctx, project); err != nil {
			respond(w, 0, nil, err)
			return
		}
		turns, err := data.ChatHistory(ctx, project, before, 21)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		var next int64
		if len(turns) > 20 {
			turns = turns[:20]
			next = turns[19].ID
		}
		slices.Reverse(turns)
		_, enabled := nodes[project]
		harness, err := data.AgentHarness(ctx, project, "management-chat")
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		err = data.ChatActivities(ctx, project, turns)
		respond(w, http.StatusOK, struct {
			Harness    string           `json:"harness"`
			Enabled    bool             `json:"enabled"`
			Messages   []store.ChatTurn `json:"messages"`
			NextBefore int64            `json:"next_before,omitempty"`
		}{harness, enabled, turns, next}, err)
	}))
	mux.HandleFunc("POST /projects/{id}/chat", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		if _, ok := nodes[project]; !ok {
			problem(w, http.StatusServiceUnavailable, "该项目尚未启用 Management 聊天。")
			return
		}
		var in struct {
			Model    string `json:"model"`
			Harness  string `json:"harness,omitempty"`
			ClientID string `json:"client_id"`
			Text     string `json:"text"`
		}
		if !decodeLimited(w, r, &in, 32<<10) {
			return
		}
		in.Text = strings.TrimSpace(in.Text)
		id, err := uuid.Parse(in.ClientID)
		if err != nil || id == uuid.Nil || in.Text == "" || strings.ContainsRune(in.Text, 0) || utf8.RuneCountInString(in.Text) > 4000 {
			problem(w, http.StatusBadRequest, "请输入 1 至 4000 个字符。")
			return
		}
		if !agents.ValidModel(in.Model) {
			problem(w, 400, "模型标识无效。")
			return
		}
		turn, err := data.SendChat(r.Context(), project, id.String(), in.Text, in.Model, in.Harness)
		respond(w, http.StatusCreated, turn, err)
	}))
	mux.HandleFunc("POST /projects/{id}/chat/actions/{action}/confirm", authorized(owner, chat.confirm))
	mux.HandleFunc("POST /internal/management-chat/claim", nodes.authorize(func(project int64, w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-ForgeOS-Claim-Key") == "" || r.Header.Get("X-ForgeOS-Node") == "" {
			problem(w, http.StatusBadRequest, "Management chat claims require a node and a claim key")
			return
		}
		idempotentRoleClaim(data, project, "management-chat", func(w http.ResponseWriter, r *http.Request) {
			var in struct{}
			if !decode(w, r, &in) {
				return
			}
			ctx := r.Context()
			job, err := data.ClaimChat(ctx, project, r.URL.Query().Get("agent_settings") == "1" && r.URL.Query().Get("supports_role_resume") == "1")
			if err != nil {
				respond(w, 0, nil, err)
				return
			}
			if job == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			job.Prompt, err = chat.prompt(ctx, *job)
			if err != nil {
				respond(w, 0, nil, err)
				return
			}
			respond(w, http.StatusOK, job, nil)
		})(w, r)
	}))
	mux.HandleFunc("POST /internal/management-chat/result", nodes.authorize(chat.result))
}

type chatProposal struct {
	Kind        string `json:"kind"`
	IssueNumber int64  `json:"issue_number"`
}

var chatIssueNumber = regexp.MustCompile(`(?i)(?:#|＃|issue\s+|议题\s*|工单\s*)([1-9][0-9]*)`)

func (c projectChat) result(project int64, w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        int64         `json:"id"`
		Lease     string        `json:"lease"`
		Answer    string        `json:"answer"`
		Citations []string      `json:"citations"`
		Action    *chatProposal `json:"action"`
		Error     string        `json:"error"`
	}
	if !decodeLimited(w, r, &in, 64<<10) {
		return
	}
	in.Answer = strings.TrimSpace(in.Answer)
	token, err := uuid.Parse(in.Lease)
	if err != nil || token == uuid.Nil || in.ID < 1 || len(in.Answer) > 16<<10 || len(in.Citations) > 24 ||
		strings.ContainsRune(in.Answer, 0) || (in.Error == "" && in.Answer == "") {
		problem(w, http.StatusBadRequest, "invalid Management chat result")
		return
	}
	ctx := r.Context()
	text, completed, err := c.data.ChatResultInput(ctx, project, in.ID, token.String())
	if err != nil || completed {
		respond(w, http.StatusNoContent, nil, err)
		return
	}
	var action *store.ChatAction
	failure := ""
	if in.Error != "" {
		in.Answer, in.Citations = "", nil
		failure = "Management 回合失败，请重试。"
	} else if in.Action != nil && in.Action.Kind != "" {
		var reason string
		action, reason = c.prepareAction(ctx, project, text, *in.Action)
		if reason != "" {
			in.Answer += "\n\n" + reason
		}
	}
	err = c.data.CompleteChat(ctx, project, in.ID, token.String(), in.Answer, failure, in.Citations, action)
	respond(w, http.StatusNoContent, nil, err)
}

func (c projectChat) prepareAction(ctx context.Context, project int64, text string, proposed chatProposal) (*store.ChatAction, string) {
	switch proposed.Kind {
	case "check_head":
		p, err := c.data.ProjectWorkspace(ctx, project)
		if err != nil {
			return nil, "暂时无法为本项目准备 HEAD 检查。"
		}
		card := &store.ChatAction{Kind: proposed.Kind, Label: "检查本项目当前 HEAD", Repository: p.Repository}
		if p.CheckStatus == "DONE" || p.CheckStatus == "FAILED" {
			card.HeadRetry = p.CheckID
		}
		return card, ""
	case "dispatch_issue":
		mentioned := slices.ContainsFunc(chatIssueNumber.FindAllStringSubmatch(text, -1), func(match []string) bool {
			id, _ := strconv.ParseInt(match[1], 10, 64)
			return id > 0 && id == proposed.IssueNumber
		})
		if !mentioned {
			return nil, "请在派发请求里写明本项目的 Issue 编号，例如「派发 #128」。"
		}
		repo, issue, err := c.readIssue(ctx, project, proposed.IssueNumber)
		if err != nil {
			return nil, "无法读取本项目的该 Issue。请核对仓库连接、权限及 Issue 是否仍然开放。"
		}
		card := &store.ChatAction{Kind: proposed.Kind, Label: issue.Title, Repository: repo.FullName,
			IssueNumber: issue.Number, RepositoryID: repo.ID, SourceDigest: issueSourceDigest(issue)}
		if store.ValidWorkType(issue.WorkType) {
			card.WorkType = issue.WorkType
		}
		return card, ""
	default:
		return nil, "项目聊天支持派发现有 Issue 和运行 HEAD 检查。"
	}
}

func (c projectChat) readIssue(ctx context.Context, project, number int64) (githubProject, store.Issue, error) {
	p, err := c.data.Project(ctx, project)
	if err != nil {
		return githubProject{}, store.Issue{}, err
	}
	return readGitHubIssue(ctx, c.root, p, c.get, number)
}

func (c projectChat) confirm(w http.ResponseWriter, r *http.Request) {
	project, ok := pathID(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("action"), 10, 64)
	if err != nil || id < 1 {
		problem(w, http.StatusBadRequest, "无效的操作卡。")
		return
	}
	var in struct {
		WorkType string `json:"work_type"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	action, err := c.data.ChatAction(ctx, project, id)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	var spec *store.IssueSpec
	if action.Status == "pending" && action.Kind == "dispatch_issue" {
		repo, issue, sourceErr := c.readIssue(ctx, project, action.IssueNumber)
		switch {
		case sourceErr == nil && repo.ID == action.RepositoryID && issueSourceDigest(issue) == action.SourceDigest:
			if action.WorkType == "" {
				if !store.ValidWorkType(in.WorkType) {
					problem(w, http.StatusConflict, "select an Issue work type before dispatch")
					return
				}
				issue.OwnerClassified = true
				if issue.WorkType == "clarification" {
					issue.ClassificationDigest = action.SourceDigest
				}
				issue.WorkType = in.WorkType
			} else if in.WorkType != "" && in.WorkType != action.WorkType {
				problem(w, http.StatusConflict, "work type differs from the current GitHub Issue")
				return
			}
			frozen := repo.spec(issue, "auto", "", validation.Policy{})
			frozen.ImportOrigin = "owner"
			spec = &frozen
		case sourceErr == nil, errors.Is(sourceErr, errNotOpen), errors.Is(sourceErr, errIdentity), errors.Is(sourceErr, errMissing):
			// A changed or unavailable source leaves a stable stale receipt.
		default:
			problem(w, http.StatusBadGateway, "暂时无法核对 GitHub Issue，请稍后重试。")
			return
		}
	}
	turn, err := c.data.ConfirmChatAction(ctx, project, id, spec)
	if err == nil && action.Kind == "check_head" && turn.Action != nil && turn.Action.Status == "confirmed" && action.Status == "pending" {
		scan, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := observeDocuments(scan, c.data, c.root, project); err != nil {
			log.Printf("project %d document scan at chat HEAD check: %v", project, err)
		}
	}
	if err == nil {
		turns := []store.ChatTurn{turn}
		err = c.data.ChatActivities(ctx, project, turns)
		turn = turns[0]
	}
	respond(w, http.StatusOK, turn, err)
}
