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

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
	"github.com/google/uuid"
)

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
	newHandler := func(nodes chatNodes) *http.ServeMux {
		mux := http.NewServeMux()
		registerProjectChat(mux, data, root, "owner", nodes, get)
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
	send := func(project int64, text string) store.ChatTurn {
		t.Helper()
		var turn, replay store.ChatTurn
		body := map[string]string{"client_id": uuid.NewString(), "text": text}
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
		path := "/internal/management-chat/claim?supports_role_resume=1"
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
			if !strings.Contains(frozen.Agent.Prompt, skill.Entry) || strings.Contains(frozen.Agent.Prompt, "Quote the verified handoff.") {
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
	body := map[string]string{"client_id": uuid.NewString(), "text": "搜索登录相关的项目资料", "model": "alternate-model", "harness": "claude"}
	request("POST", chat(alpha.ID), "owner", "", body, 201, &query)
	if query.RequestedModel != "alternate-model" {
		t.Fatal("message lost selected model")
	}
	body["model"] = "other-model"
	request("POST", chat(alpha.ID), "owner", "", body, 409, nil)
	request("POST", "/internal/management-chat/claim?supports_role_resume=1", "alpha-worker", strings.Repeat("b", 32), struct{}{}, 204, nil)
	request("POST", "/internal/management-chat/claim", "owner", strings.Repeat("a", 32), struct{}{}, 401, nil)
	request("POST", "/internal/management-chat/claim", "beta-worker", strings.Repeat("a", 32), struct{}{}, 204, nil)
	request("POST", "/internal/management-chat/claim?supports_role_resume=1&agent_settings=1&role_harnesses=codex", "alpha-worker", strings.Repeat("c", 32), struct{}{}, 204, nil)
	job := claim("alpha-worker", query)
	if !strings.Contains(job.Prompt, "AlphaLoginEvidence") || strings.Contains(job.Prompt, "PrivateBetaLoginEvidence") {
		t.Fatal("chat context did not stay in the selected project")
	}
	keys := []string{"material:login-notes", fmt.Sprintf("task:%d", betaTask.ID), "https://host.example/forged"}
	result(job, "beta-worker", nil, keys, 409)
	result(job, "alpha-worker", nil, keys, 204)
	result(job, "alpha-worker", nil, keys, 204)
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
}
