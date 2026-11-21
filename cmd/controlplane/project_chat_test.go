package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/chatkernel"
	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

type testChatProviders map[string]chatkernel.Provider

func (v testChatProviders) ChatProvider(ctx context.Context, id int64) (chatkernel.Provider, error) {
	return v[fmt.Sprint(id)], nil
}
func (v testChatProviders) ModelConnection(ctx context.Context, id uuid.UUID) (chatkernel.Provider, error) {
	return v[id.String()], nil
}
func (v testChatProviders) SetModelConnection(ctx context.Context, id uuid.UUID, p chatkernel.Provider) error {
	v[id.String()] = p
	return nil
}

func TestProjectChatWorkflow(t *testing.T) {
	t.Setenv("FORGEOS_SKILLS_DIR", t.TempDir())
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root, alphaDir, _ := issueTestRepo(t)
	betaDir := filepath.Join(root, "beta")
	if _, err := localrepo.Git(ctx, root, "init", "--quiet", betaDir); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"remote", "add", "origin", "https://github.com/my-org/beta"},
		{"-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "--allow-empty", "-qm", "base"},
	} {
		if _, err := localrepo.Git(ctx, betaDir, args...); err != nil {
			t.Fatal(err)
		}
	}
	alpha, err := data.CreateProject(ctx, alphaDir)
	if err != nil {
		t.Fatal(err)
	}
	beta, err := data.CreateProject(ctx, betaDir)
	if err != nil {
		t.Fatal(err)
	}
	betaTask, err := data.ProjectCheck(ctx, beta.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	alphaTask, err := data.ProjectCheck(ctx, alpha.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE work_items SET status='DONE' WHERE id=$1", alphaTask.ID); err != nil {
		t.Fatal(err)
	}
	for n, name := range map[int64]string{alpha.ID: "AlphaLoginEvidence", beta.ID: "PrivateBetaLoginEvidence"} {
		if _, err := data.SaveMaterial(ctx, n, "team-convention", store.MaterialInput{Title: "Team convention", Source: "owner", Kind: "memory", Content: name + "-persistent-memory"}); err != nil {
			t.Fatal(err)
		}
		if _, err := data.SaveMaterial(ctx, n, "login-notes", store.MaterialInput{Title: name, Source: "lab", Kind: "material",
			Content: "登录方式与部署步骤"}); err != nil {
			t.Fatal(err)
		}
	}
	title := "Alpha #7"
	get := func(_ context.Context, path string, dest any) error {
		var response any
		switch path {
		case "repos/my-org/project":
			response = map[string]any{"id": 88, "full_name": "my-org/project"}
		case "repos/my-org/beta":
			response = map[string]any{"id": 89, "full_name": "my-org/beta"}
		case "repositories/88/issues/7", "repositories/89/issues/7":
			slug, text, id := "my-org/project", title, 801
			if strings.Contains(path, "/89/") {
				slug, text, id = "my-org/beta", "Beta #7", 802
			}
			response = map[string]any{"id": id, "number": 7, "title": text, "body": "Update the docs", "state": "open", "updated_at": "2026-01-05T16:00:00Z",
				"repository_url": "https://api.github.com/repos/" + slug, "html_url": "https://github.com/" + slug + "/issues/7"}
		default:
			t.Fatalf("unexpected GitHub read %q", path)
		}
		raw, _ := json.Marshal(response)
		return json.Unmarshal(raw, dest)
	}
	nodes := chatNodes{alpha.ID: sha256.Sum256([]byte("alpha-worker")), beta.ID: sha256.Sum256([]byte("beta-worker"))}
	providers := testChatProviders{}
	var issueClient chatIssueClient
	newHandler := func(nodes chatNodes) *http.ServeMux {
		mux := http.NewServeMux()
		registerProjectChat(mux, data, root, "owner", nodes, get, providers, issueClient)
		registerAgentSettings(mux, data, "owner", legacyWorker("worker"))
		return mux
	}
	handler := newHandler(nodes)
	request := func(method, path, token, key string, body any, expected int, dest any) []byte {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		if key != "" {
			req.Header.Set("X-ForgeOS-Claim-Key", key)
			req.Header.Set("X-ForgeOS-Node", "test-node")
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != expected {
			t.Fatalf("%s %s = %d, expected %d: %s", method, path, res.Code, expected, res.Body)
		}
		if dest != nil {
			if err := json.Unmarshal(res.Body.Bytes(), dest); err != nil {
				t.Fatal(err)
			}
		}
		return res.Body.Bytes()
	}
	chat := func(project int64) string { return fmt.Sprintf("/projects/%d/chat", project) }
	var profile, second store.ChatModel
	send := func(project int64, text string) store.ChatTurn {
		t.Helper()
		var turn, replay store.ChatTurn
		body := map[string]string{"client_id": uuid.NewString(), "text": text, "profile_id": second.ID.String()}
		request("POST", chat(project), "owner", "", body, 201, &turn)
		request("POST", chat(project), "owner", "", body, 201, &replay)
		if replay.ID != turn.ID {
			t.Fatal("message retry created a duplicate turn")
		}
		return turn
	}
	var skill store.SkillOption
	claim := func(token string, want store.ChatTurn) store.ChatJob {
		t.Helper()
		var job store.ChatJob
		key := strings.ReplaceAll(uuid.NewString(), "-", "")
		path := "/internal/management-chat/claim?project_kernel=1&supports_role_resume=1"
		if want.RequestedModel != "" {
			path += "&agent_settings=1&role_harnesses=codex,claude"
		}
		first := request("POST", path, token, key, struct{}{}, 200, &job)
		if want.RequestedModel != "" {
			var frozen struct {
				Agent struct{ Model, Harness, Prompt string }
			}
			if json.Unmarshal(first, &frozen) != nil || frozen.Agent.Model != want.RequestedModel || frozen.Agent.Harness != want.RequestedHarness {
				t.Fatal("selected model did not reach the frozen assignment")
			}
			if token == "alpha-worker" && (!strings.Contains(frozen.Agent.Prompt, skill.Entry) || strings.Contains(frozen.Agent.Prompt, "Quote the verified handoff.")) {
				t.Fatal("selected skill missing from chat harness input")
			}
		}
		second := request("POST", path, token, key, struct{}{}, 200, nil)
		if !bytes.Equal(first, second) || job.ID != want.ID || !job.Recover {
			t.Fatal("claim was not scoped, frozen and replayable")
		}
		return job
	}
	result := func(job store.ChatJob, token string, proposal *chatProposal, cites []string, expected int) {
		t.Helper()
		request("POST", "/internal/management-chat/result", token, "", map[string]any{"id": job.ID, "lease": job.Lease,
			"answer": "根据本项目记录。", "citations": cites, "action": proposal}, expected, nil)
	}
	var library []store.SkillOption
	request("GET", "/projects/agent-skills", "owner", "", nil, 200, &library)
	if len(library) != 0 {
		t.Fatal("unexpected preinstalled Skill")
	}
	var upload bytes.Buffer
	form := multipart.NewWriter(&upload)
	for path, contents := range map[string]string{"SKILL.md": "---\nname: chat-handoff\ndescription: Handoff notes\n---\nQuote the verified handoff.", "references/notes.txt": "example reference"} {
		part, err := form.CreateFormFile(path, filepath.Base(path))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/projects/agent-skills", &upload)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer owner")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &skill) != nil || skill.FileCount != 2 {
		t.Fatalf("directory upload failed: %d %s", response.Code, response.Body)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(skill.Entry), "references", "notes.txt"))
	if err != nil || string(raw) != "example reference" {
		t.Fatal("missing imported reference")
	}

	request("PUT", fmt.Sprintf("/projects/%d/agents", alpha.ID), "owner", "", map[string]any{
		"key": "management-chat", "previous": 0, "effective": 0, "settings": map[string]any{
			"prompt": "Chat with the project owner.", "harness": "codex", "model": "", "skills": []int64{skill.ID},
		},
	}, 204, nil)
	var query store.ChatTurn
	body := map[string]string{"client_id": uuid.NewString(), "text": "搜索登录相关的项目资料"}
	request("POST", chat(alpha.ID), "owner", "", body, 409, nil)
	const catalog = "/projects/chat-models"
	t.Run("test a draft and a saved model without changing credentials", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/chat/completions" || r.Method != "POST" {
				t.Errorf("incorrect probe route: %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer local-fixture" {
				w.WriteHeader(401)
				fmt.Fprint(w, "private error text")
				return
			}
			var in struct {
				Model  string
				Stream bool
				Format struct{ Type string } `json:"response_format"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !in.Stream || in.Format.Type != "json_schema" {
				t.Error("invalid probe payload")
			}
			if in.Model == "malformed" {
				fmt.Fprint(w, `{"choices":[]}`)
				return
			}
			fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}]}`)
		}))
		defer upstream.Close()
		testURL := catalog + "/test"
		cfg := map[string]string{"model": "probe", "base_url": upstream.URL + "/v1", "api_key": "local-fixture", "output_format": "json_schema"}
		request("POST", testURL, "alpha-worker", "", cfg, 401, nil)
		request("POST", testURL, "owner", "", cfg, 204, nil)
		cfg["model"] = "malformed"
		request("POST", testURL, "owner", "", cfg, 502, nil)
		cfg["model"] = "probe"
		cfg["api_key"] = "wrong-key"
		msg := request("POST", testURL, "owner", "", cfg, 502, nil)
		if !bytes.Contains(msg, []byte("401")) || bytes.Contains(msg, []byte("private error text")) || bytes.Contains(msg, []byte("wrong-key")) {
			t.Fatal("invalid diagnostic")
		}
		cfg["api_key"] = ""
		request("POST", testURL, "owner", "", cfg, 400, nil)
		var list []store.ChatModel
		request("GET", catalog, "owner", "", nil, 200, &list)
		if len(list) != 0 {
			t.Fatal("probe activated a model")
		}
		cfg["api_key"] = "local-fixture"
		var saved store.ChatModel
		request("POST", catalog, "owner", "", cfg, 200, &saved)
		cfg["id"], cfg["revision"], cfg["api_key"] = saved.ID.String(), saved.Revision.String(), ""
		request("POST", testURL, "owner", "", cfg, 204, nil)
		cfg["base_url"] = upstream.URL + "/other"
		request("POST", testURL, "owner", "", cfg, 400, nil)
		request("DELETE", catalog, "owner", "", map[string]string{"id": cfg["id"], "revision": cfg["revision"]}, 204, nil)
		request("POST", testURL, "owner", "", cfg, 409, nil)
	})
	// Simulate the migration marker created for a project predating this schema.
	providers[fmt.Sprint(alpha.ID)] = chatkernel.Provider{BaseURL: "http://127.0.0.1:8020/v1", Model: "legacy", APIKey: "legacy-key"}
	if _, err := db.ExecContext(ctx, "INSERT INTO chat_models(legacy_project) VALUES($1),($2)", alpha.ID, beta.ID); err != nil {
		t.Fatal(err)
	}
	var migrated []store.ChatModel
	request("GET", catalog, "owner", "", nil, 200, &migrated)
	if len(migrated) != 1 || migrated[0].Model != "legacy" || providers[migrated[0].Revision.String()].APIKey != "legacy-key" {
		t.Fatal("legacy model import failed")
	}
	request("DELETE", catalog, "owner", "", map[string]any{"id": migrated[0].ID, "revision": migrated[0].Revision}, 204, nil)
	fields := map[string]string{"model": "alternate-model", "base_url": "http://127.0.0.1:8010/v1", "name": "设计"}
	request("POST", catalog, "owner", "", fields, 400, nil)
	fields["api_key"] = "   "
	request("POST", catalog, "owner", "", fields, 400, nil)
	request("GET", catalog, "owner", "", nil, 200, &migrated)
	if len(migrated) != 0 {
		t.Fatal("keyless model in active catalog")
	}
	fields["api_key"] = "fixture-secret"
	request("POST", catalog, "owner", "", fields, 200, &profile)
	if profile.Name != fields["model"] {
		t.Fatal("role alias exposed as model name")
	}
	if _, err := db.ExecContext(ctx, "UPDATE chat_models SET name='旧管理名称' WHERE id=$1", profile.ID); err != nil {
		t.Fatal(err)
	}
	fields["model"] = "second-model"
	fields["api_key"] = "second-key"
	request("POST", catalog, "owner", "", fields, 200, &second)
	var models []store.ChatModel
	catalogJSON := request("GET", catalog, "owner", "", nil, 200, &models)
	if len(models) != 2 || bytes.Contains(catalogJSON, []byte("旧管理名称")) || bytes.Contains(catalogJSON, []byte("fixture-secret")) || bytes.Contains(catalogJSON, []byte("api_key")) {
		t.Fatal("invalid public catalog")
	}
	body["profile_id"] = profile.ID.String()
	request("POST", chat(alpha.ID), "owner", "", body, 201, &query)
	if query.RequestedModel != "alternate-model" {
		t.Fatal("wrong model recorded")
	}
	fields["id"] = profile.ID.String()
	fields["revision"] = profile.Revision.String()
	fields["api_key"] = "rotated-key"
	request("PUT", catalog, "owner", "", fields, 200, nil)
	body["profile_id"] = second.ID.String()
	request("POST", chat(alpha.ID), "owner", "", body, 409, nil)
	request("POST", "/internal/management-chat/claim?supports_role_resume=1", "alpha-worker", strings.Repeat("b", 32), struct{}{}, 204, nil)
	request("POST", "/internal/management-chat/claim", "owner", strings.Repeat("a", 32), struct{}{}, 401, nil)
	request("POST", "/internal/management-chat/claim", "beta-worker", strings.Repeat("a", 32), struct{}{}, 204, nil)
	request("POST", "/internal/management-chat/claim?supports_role_resume=1&agent_settings=1&role_harnesses=codex", "alpha-worker", strings.Repeat("c", 32), struct{}{}, 204, nil)
	job := claim("alpha-worker", query)
	leaseRequest := map[string]any{"id": job.ID, "lease": job.Lease}
	draft := map[string]any{"id": job.ID, "lease": job.Lease, "answer": "正在生成正文"}
	request("POST", "/internal/management-chat/progress", "beta-worker", "", draft, 409, nil)
	request("POST", "/internal/management-chat/progress", "alpha-worker", "", draft, 204, nil)
	interim, err := data.ChatTurn(ctx, alpha.ID, job.ID)
	if err != nil || interim.Status != "running" || interim.Answer != "正在生成正文" || interim.Action != nil {
		t.Fatal("missing in-progress snapshot")
	}

	var activeProvider chatkernel.Provider
	request("POST", "/internal/management-chat/provider", "beta-worker", "", leaseRequest, 409, nil)
	request("POST", "/internal/management-chat/provider", "alpha-worker", "", leaseRequest, 200, &activeProvider)
	if activeProvider.APIKey != "fixture-secret" || activeProvider.Model != query.RequestedModel {
		t.Fatal("active project lease did not resolve its model credentials")
	}
	if !strings.Contains(job.Prompt, "AlphaLoginEvidence-persistent-memory") || strings.Contains(job.Prompt, "PrivateBetaLoginEvidence-persistent-memory") {
		t.Fatal("durable memory must be recalled without keywords and isolated by project")
	}
	if !strings.Contains(job.Prompt, "AlphaLoginEvidence") || strings.Contains(job.Prompt, "PrivateBetaLoginEvidence") {
		t.Fatal("chat context did not stay in the selected project")
	}
	keys := []string{"material:login-notes", fmt.Sprintf("task:%d", betaTask.ID), "https://host.example/forged"}
	result(job, "beta-worker", nil, keys, 409)
	result(job, "alpha-worker", nil, keys, 204)
	result(job, "alpha-worker", nil, keys, 204)
	request("POST", "/internal/management-chat/provider", "alpha-worker", "", leaseRequest, 409, nil)
	request("POST", "/internal/management-chat/progress", "alpha-worker", "", draft, 409, nil)
	turn, err := data.ChatTurn(ctx, alpha.ID, query.ID)
	if err != nil || len(turn.Sources) != 1 || turn.Sources[0].Key != "material:login-notes" {
		t.Fatalf("chat did not keep only authorized citations: %+v %v", turn.Sources, err)
	}

	propose := func(project int64, token, text string, proposal chatProposal) store.ChatTurn {
		t.Helper()
		turn := send(project, text)
		result(claim(token, turn), token, &proposal, nil, 204)
		prepared, err := data.ChatTurn(ctx, project, turn.ID)
		if err != nil || prepared.Action == nil || prepared.Action.Status != "pending" {
			t.Fatalf("confirmation card missing: %+v %v", prepared, err)
		}
		return prepared
	}
	confirm := func(project int64, action int64, selection any, expected int) store.ChatTurn {
		t.Helper()
		var saved store.ChatTurn
		var dest any
		if expected == 200 {
			dest = &saved
		}
		request("POST", fmt.Sprintf("%s/actions/%d/confirm", chat(project), action), "owner", "", selection, expected, dest)
		return saved
	}
	issue := chatProposal{Kind: "dispatch_issue", IssueNumber: 7}
	first := propose(alpha.ID, "alpha-worker", "派发 Issue #7", issue)
	other := propose(beta.ID, "beta-worker", "派发 Issue #7", issue)
	if first.Action.Repository == other.Action.Repository || first.Action.Label == other.Action.Label {
		t.Fatal("same issue number was not resolved in its own repository")
	}
	confirm(beta.ID, first.Action.ID, struct{}{}, 404)
	title = "Alpha #7 revised"
	stale := confirm(alpha.ID, first.Action.ID, map[string]string{"work_type": "development"}, 200)
	if stale.Action.Status != "stale" || stale.Action.TaskID != 0 {
		t.Fatal("stale source was dispatched")
	}
	fresh := propose(alpha.ID, "alpha-worker", "派发更新后的 #7", issue)
	confirm(alpha.ID, fresh.Action.ID, struct{}{}, 409)
	saved := confirm(alpha.ID, fresh.Action.ID, map[string]string{"work_type": "development"}, 200)
	repeated := confirm(alpha.ID, fresh.Action.ID, struct{}{}, 200)
	if saved.Action.TaskID < 1 || saved.Action.TaskID != repeated.Action.TaskID || len(saved.Activities) == 0 {
		t.Fatal("confirmed issue is missing a durable receipt or its real activity")
	}
	confirmed := confirm(beta.ID, other.Action.ID, map[string]string{"work_type": "code-review"}, 200)
	task, err := data.Task(ctx, confirmed.Action.TaskID)
	if err != nil || task.ProjectID != beta.ID || task.Spec.Mode != "plan" || task.Spec.Issue.RepositoryID != 89 {
		t.Fatalf("Beta issue entered the wrong execution chain: %+v %v", task, err)
	}
	head := propose(alpha.ID, "alpha-worker", "运行 HEAD 检查", chatProposal{Kind: "check_head"})
	headSaved := confirm(alpha.ID, head.Action.ID, struct{}{}, 200)
	if headSaved.Action.TaskID <= alphaTask.ID || headSaved.Action.TaskID != confirm(alpha.ID, head.Action.ID, struct{}{}, 200).Action.TaskID {
		t.Fatal("HEAD check did not create and replay the same action receipt")
	}
	handler = newHandler(nil)
	var page struct {
		Enabled  bool
		Messages []store.ChatTurn
	}
	request("GET", chat(alpha.ID), "owner", "", nil, 200, &page)
	if page.Enabled || len(page.Messages) != 4 || page.Messages[0].ID != query.ID || page.Messages[3].Action.TaskID != headSaved.Action.TaskID {
		t.Fatalf("history lost its project scope or saved state on restart: %+v", page)
	}
	t.Run("create and dispatch an edited chat draft", func(t *testing.T) {
		var raw githubIssue
		var posts int
		const endpoint = "repos/my-org/project/issues"
		issueClient = func(ctx context.Context, p store.Project) (deliveryAPI, error) {
			if p.ID != alpha.ID {
				t.Fatal("wrong project")
			}
			return func(ctx context.Context, method, path string, body, dest any) error {
				switch {
				case method == "POST" && path == endpoint:
					posts++
					if posts == 1 {
						return issueAPIStatus(403)
					}
					draft := body.(map[string]string)
					raw = githubIssue{ID: 902, Number: 22, State: "open", Title: draft["title"], Body: draft["body"], UpdatedAt: "2026-01-06T10:00:00Z", HTMLURL: "https://github.com/my-org/project/issues/22", RepoURL: "https://api.github.com/repos/my-org/project"}
					if raw.Title != "Owner 修改的标题" || !strings.HasPrefix(raw.Body, "Owner 修改的验收标准\n\n<!-- forgeos-chat-issue:") {
						t.Fatal("published unapproved draft")
					}
					// The remote committed the write, but its reply was lost.
					return context.DeadlineExceeded
				case method == "GET" && strings.HasPrefix(path, endpoint+"?"):
					*(dest.(*[]githubIssue)) = []githubIssue{raw}
					return nil
				case method == "GET" && path == endpoint+"/22":
					*(dest.(*githubIssue)) = raw
					return nil
				case method == "GET":
					return get(ctx, path, dest)
				default:
					t.Fatalf("unexpected mutation %s %s", method, path)
					return nil
				}
			}, nil
		}
		handler = newHandler(nodes)
		proposed := propose(alpha.ID, "alpha-worker", "需求：增加筛选按钮", chatProposal{Kind: "create_issue", Title: "增加筛选", Body: "需求与验收标准", WorkType: "development"})
		if posts != 0 || proposed.Action.Body != "需求与验收标准" || proposed.Action.IssueNumber != 0 {
			t.Fatal("draft must not publish")
		}
		edits := chatIssueDraft{Title: "Owner 修改的标题", Body: "Owner 修改的验收标准", WorkType: "testing"}
		id := proposed.Action.ID
		draftRef, err := data.ResolveChatTask(ctx, alpha.ID, proposed.ID+1, store.TaskReference{By: "recent"})
		if err != nil || draftRef.Result != "not_dispatched" {
			t.Fatalf("pending draft treated as dispatched: %+v %v", draftRef, err)
		}
		confirm(beta.ID, id, edits, 404)
		request("POST", fmt.Sprintf("%s/actions/%d/confirm", chat(alpha.ID), id), "alpha-worker", "", edits, 401, nil)
		confirm(alpha.ID, id, edits, 502)
		afterReject, err := data.ChatAction(ctx, alpha.ID, id)
		if err != nil || afterReject.CreationStarted != nil || afterReject.WorkType != "testing" {
			t.Fatal("definite rejection did not unlock draft")
		}
		confirm(alpha.ID, id, edits, 502)
		// A local dispatch error after successful reconciliation must retain #22.
		if _, err = db.ExecContext(ctx, `CREATE FUNCTION fail_chat_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'temporary'; END $$; CREATE TRIGGER fail_chat_test BEFORE INSERT ON work_items FOR EACH ROW EXECUTE FUNCTION fail_chat_test()`); err != nil {
			t.Fatal(err)
		}
		confirm(alpha.ID, id, chatIssueDraft{}, 502)
		partial, err := data.ChatAction(ctx, alpha.ID, id)
		if err != nil || partial.IssueNumber != 22 || partial.TaskID != 0 || partial.Status != "pending" {
			t.Fatal("lost remote receipt")
		}
		if _, err = db.ExecContext(ctx, "DROP TRIGGER fail_chat_test ON work_items"); err != nil {
			t.Fatal(err)
		}
		done := confirm(alpha.ID, id, chatIssueDraft{}, 200)
		again := confirm(alpha.ID, id, edits, 200)
		if done.Action.Status != "confirmed" || done.Action.TaskID == 0 || done.Action.TaskID != again.Action.TaskID || posts != 2 {
			t.Fatal("duplicate issue or task")
		}
		task, err := data.Task(ctx, done.Action.TaskID)
		if err != nil || task.Spec.Issue.Number != 22 || task.Spec.Issue.WorkType != "testing" || task.Spec.Mode != "plan" {
			t.Fatal("incorrect dispatched task")
		}
		for i := 0; i < 6; i++ {
			if _, err := db.ExecContext(ctx, "INSERT INTO project_chat_turns(project_id,client_id,text,answer,status) VALUES($1,$2,'无关话题','无任务引用','completed')", alpha.ID, uuid.NewString()); err != nil {
				t.Fatal(err)
			}
		}
		job := claim("alpha-worker", send(alpha.ID, "刚才那个任务做到哪了"))
		lookup := func(node, name string, args any, status int, out any) {
			t.Helper()
			request("POST", "/internal/management-chat/task-tool", node, "", map[string]any{
				"id": job.ID, "lease": job.Lease, "name": name, "arguments": args,
			}, status, out)
		}
		for _, ref := range []store.TaskReference{{By: "recent"}, {By: "latest_dispatch"}, {By: "issue_number", Value: "22"}, {By: "title", Value: "修改的标题"}, {By: "task_id", Value: fmt.Sprint(task.ID)}} {
			var found struct{ Data store.ChatTaskMatches }
			lookup("alpha-worker", chatkernel.ResolveTask, ref, 200, &found)
			if found.Data.Result != "resolved" || found.Data.Matches[0].TaskID != task.ID {
				t.Fatalf("incorrect reference for %v: %+v", ref, found)
			}
		}
		lookup("beta-worker", chatkernel.TaskProgress, map[string]int64{"task_id": task.ID}, 409, nil)
		foreign, err := data.CreateTask(ctx, beta.ID)
		if err != nil {
			t.Fatal(err)
		}
		lookup("alpha-worker", chatkernel.TaskProgress, map[string]int64{"task_id": foreign.ID}, 404, nil)
		var progress struct {
			Data    struct{ Status string }
			Sources []store.ChatSource
		}
		lookup("alpha-worker", chatkernel.TaskProgress, map[string]int64{"task_id": task.ID}, 200, &progress)
		if progress.Data.Status != task.Status || len(progress.Sources) != 1 || progress.Sources[0].Key != store.ChatTaskSource(task.ID).Key {
			t.Fatal("missing current task or citation")
		}
		result(job, "alpha-worker", nil, []string{progress.Sources[0].Key}, 204)
		lookup("alpha-worker", chatkernel.TaskProgress, map[string]int64{"task_id": task.ID}, 409, nil)
		if _, err = db.ExecContext(ctx, "UPDATE work_items SET status='BLOCKED' WHERE id=$1", task.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = db.ExecContext(ctx, "INSERT INTO work_dependencies(task_id,prerequisite) VALUES($1,$2)", task.ID, alphaTask.ID); err != nil {
			t.Fatal(err)
		}
		snapshot, _, err := data.ChatTaskProgress(ctx, alpha.ID, task.ID)
		var blocked struct {
			Dependencies []struct {
				Prerequisite int64
				TaskStatus   string `json:"task_status"`
			} `json:"pending_dependencies"`
		}
		if err != nil || json.Unmarshal(snapshot, &blocked) != nil || len(blocked.Dependencies) != 1 || blocked.Dependencies[0].Prerequisite != alphaTask.ID || blocked.Dependencies[0].TaskStatus != "DONE" {
			t.Fatal("lost outstanding delivery dependency", err)
		}
		// A turn that discussed two tasks must not silently pick one for "that task".
		var conversationID int64
		err = db.QueryRowContext(ctx, "INSERT INTO project_chat_turns(project_id,client_id,text,answer,status,progress_task_ids) VALUES($1,$2,'比较两项任务','对比结果','completed',ARRAY[$3::bigint,$4::bigint]) RETURNING id", alpha.ID, uuid.NewString(), task.ID, alphaTask.ID).Scan(&conversationID)
		if err != nil {
			t.Fatal(err)
		}
		both, err := data.ResolveChatTask(ctx, alpha.ID, conversationID+1, store.TaskReference{By: "recent"})
		if err != nil || both.Result != "ambiguous" || len(both.Matches) != 2 {
			t.Fatal("lost ambiguous reference", both, err)
		}
		last, err := data.ResolveChatTask(ctx, alpha.ID, conversationID+1, store.TaskReference{By: "latest_dispatch"})
		if err != nil || last.Result != "resolved" || last.Matches[0].TaskID != task.ID {
			t.Fatal("discussion displaced last dispatch", last, err)
		}
	})

	t.Run("project plan dispatch preserves task and Goal modes", func(t *testing.T) {
		var posts int64
		issueClient = func(ctx context.Context, p store.Project) (deliveryAPI, error) {
			return func(ctx context.Context, method, path string, body, dest any) error {
				if method == "GET" {
					return get(ctx, path, dest)
				}
				posts++
				draft := body.(map[string]string)
				n := 100 + posts
				*(dest.(*githubIssue)) = githubIssue{ID: 1000 + n, Number: n, State: "open", Title: draft["title"], Body: draft["body"], UpdatedAt: "2026-01-06T10:00:00Z",
					HTMLURL: fmt.Sprintf("https://github.com/my-org/project/issues/%d", n), RepoURL: "https://api.github.com/repos/my-org/project"}
				return nil
			}, nil
		}
		handler = newHandler(nodes)
		path := fmt.Sprintf("/projects/%d/planning", alpha.ID)
		var state struct {
			Revision   int64
			Milestones []struct {
				ID         int64
				Deliveries []struct {
					ID    int64
					Tasks []struct{ ID int64 }
				}
			}
			Drafts []struct{ ID int64 }
		}
		edit := func(e store.PlanningEdit) {
			t.Helper()
			e.Revision = &state.Revision
			request("POST", path, "owner", "", e, 200, &state)
		}
		edit(store.PlanningEdit{Action: "milestone", Title: "Release"})
		edit(store.PlanningEdit{Action: "delivery", Target: state.Milestones[0].ID, Title: "Ship", Body: "Check version"})
		d := state.Milestones[0].Deliveries[0].ID
		for _, mode := range []string{"", "plan"} {
			edit(store.PlanningEdit{Action: "draft", ID: d, Title: "Release work", Body: "Check version", Flag: mode == "plan"})
			if len(state.Drafts) != 1 {
				t.Fatal("missing pending dispatch")
			}
			a := state.Drafts[0].ID
			done := confirm(alpha.ID, a, chatIssueDraft{}, 200)
			again := confirm(alpha.ID, a, chatIssueDraft{}, 200)
			if again.Action.TaskID != done.Action.TaskID {
				t.Fatal("repeated dispatch created another task")
			}
			task, err := data.Task(ctx, done.Action.TaskID)
			if err != nil || task.Spec.Mode != mode {
				t.Fatal("incorrect execution mode", mode, err)
			}
			request("GET", path, "owner", "", nil, 200, &state)
			tasks := state.Milestones[0].Deliveries[0].Tasks
			if tasks[len(tasks)-1].ID != task.ID || len(state.Drafts) != 0 {
				t.Fatal("missing milestone task", task.ID)
			}
			var backlinks []struct{ ID int64 }
			request("GET", fmt.Sprintf("%s/tasks/%d", path, task.ID), "owner", "", nil, 200, &backlinks)
			if len(backlinks) != 1 || backlinks[0].ID != state.Milestones[0].ID {
				t.Fatal("missing planning backlink")
			}
		}
		if posts != 2 {
			t.Fatal("unexpected external writes", posts)
		}
	})
}
