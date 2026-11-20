package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestProjectTaskCategories(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(ctx, "categorized")
	if err != nil {
		t.Fatal(err)
	}
	other, err := data.CreateProject(ctx, "separate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO work_items(project_id,kind,status) VALUES
			($1,'git_head','DONE'),($1,'environment_check','DONE'),($2,'git_head','DONE');
	`, project.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	var goal, child int64
	if err := db.QueryRowContext(ctx, `
		WITH goal AS (
			INSERT INTO work_items(project_id,kind,status,github_issue_id,spec)
			VALUES ($1,'codex_issue','ACTIVE',1,'{"issue":{"title":"Review goal","work_type":"code-review"}}')
			RETURNING id
		), plan AS (
			INSERT INTO plan_revisions(task_id,revision,plan) SELECT id,1,'{}' FROM goal RETURNING task_id
		), child AS (
			INSERT INTO work_items(project_id,kind,status,spec,goal_id,plan_revision,plan_key)
			SELECT $1,'codex_issue','READY','{"issue":{"title":"Review child"}}',task_id,1,'first' FROM plan
			RETURNING id,goal_id
		)
		SELECT goal_id,id FROM child
	`, project.ID).Scan(&goal, &child); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO work_items(project_id,kind,status,github_issue_id,spec) VALUES
			($1,'codex_issue','READY',2,'{"issue":{"title":"Legacy development"}}'),
			($1,'codex_issue','READY',3,'{"issue":{"title":"Test fixtures","work_type":"testing"}}'),
			($1,'codex_issue','READY',4,'{"issue":{"title":"Investigate startup","work_type":"investigation"}}'),
			($1,'codex_issue','READY',5,'{"issue":{"title":"Prepare release","work_type":"operations"}}')
	`, project.ID); err != nil {
		t.Fatal(err)
	}
	handler := routes(data, t.TempDir(), "owner", legacyWorker("worker"), nil)
	read := func(query string, status int) store.TaskPage {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tasks?project_id=%d&%s", project.ID, query), nil)
		req.Header.Set("Authorization", "Bearer owner")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("GET %s: %d != %d: %s", query, response.Code, status, response.Body)
		}
		var page store.TaskPage
		if status == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
		}
		return page
	}
	for category, title := range map[string]string{
		"development": "Legacy development", "testing": "Test fixtures", "investigation": "Investigate startup",
		"operations": "Prepare release", "git-head": "Git HEAD 检查", "environment-check": "本地环境启动检查",
	} {
		page := read("category="+category, 200)
		if len(page.Items) != 1 || page.Items[0].Title != title || page.CategoryCounts[category] != 1 || page.Counts["ALL"] != 8 {
			t.Fatalf("%s: wrong category page %+v", category, page)
		}
	}
	page := read("category=code-review&limit=1", 200)
	if len(page.Items) != 1 || page.Items[0].ID != child || page.Items[0].GoalID != goal ||
		page.NextBefore != child || page.CategoryCounts["code-review"] != 2 {
		t.Fatalf("wrong first review page %+v", page)
	}
	page = read(fmt.Sprintf("category=code-review&limit=1&before=%d", child), 200)
	if len(page.Items) != 1 || page.Items[0].ID != goal || page.NextBefore != 0 {
		t.Fatalf("wrong next review page %+v", page)
	}
	page = read("category=code-review&status=READY&q=child", 200)
	if len(page.Items) != 1 || page.Items[0].ID != child || page.CategoryCounts["code-review"] != 1 {
		t.Fatalf("review filters returned %+v", page)
	}
	read("category=unknown", 400)
}

func TestProjectWorkbenchReadsStayScoped(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	data := store.New(db)
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	alpha, err := data.CreateProject(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	beta, err := data.CreateProject(ctx, "beta")
	if err != nil {
		t.Fatal(err)
	}
	head, err := data.CreateTask(ctx, alpha.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.Claim(ctx, "local-worker", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	issue, err := data.ImportIssue(ctx, alpha.ID, store.IssueSpec{
		Issue:      store.Issue{ID: 10, Number: 4, RepositoryID: 77, Repository: "acme/example", Title: "Candidate"},
		BaseCommit: strings.Repeat("a", 40),
		Validation: "docs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE work_items SET status='WAITING_ACCEPTANCE' WHERE id=$1", issue.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `WITH task AS (
		INSERT INTO work_items(project_id,status,kind,github_issue_id,spec)
		VALUES ($1,'WAITING_INPUT','codex_issue',11,'{"issue":{"title":"Pending question"}}') RETURNING id
	), run AS (
		INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until)
		SELECT id,'runner','/work','',now() FROM task RETURNING id,task_id
	), question AS (
		INSERT INTO decision_questions(task_id,execution_id,plan_revision,commit_sha,request_key,question,evidence,requester)
		SELECT task_id,id,0,repeat('a',40),'wording','Which title?','Candidate diff','owner' FROM run
		RETURNING id,task_id,execution_id
	) INSERT INTO task_events(task_id,execution_id,type,question_id)
	SELECT task_id,execution_id,'decision_requested',id FROM question`, beta.ID); err != nil {
		t.Fatal(err)
	}
	commit := strings.Repeat("b", 40)
	if _, err := db.ExecContext(ctx, `INSERT INTO code_reviews(execution_id,task_id,base_commit,commit_sha,requirement)
		SELECT execution_id,task_id,repeat('a',40),$1,'Check the wording' FROM decision_questions`, commit); err != nil {
		t.Fatal(err)
	}
	for _, project := range []struct{ id, count int64 }{{alpha.ID, 1}, {beta.ID, 31}} {
		for i := int64(0); i < project.count; i++ {
			proposal := store.DecisionProposal{RequestKey: fmt.Sprintf("draft-%d", i), Statement: "Review the guide",
				Source: "handbook", Validity: "current release", EvidenceRefs: []string{"guide.md"}}
			if _, err := data.ProposeDecision(ctx, project.id, proposal); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Setenv("FORGEOS_REVIEWER_PROJECT_ID", "")
	t.Setenv("FORGEOS_REVIEWER_TOKEN", "")
	root, workers := t.TempDir(), legacyWorker("worker")
	handler := routes(data, root, "owner", workers, nil)
	if err := registerCodeReviews(handler, data, root, "owner", "", workers); err != nil {
		t.Fatal(err)
	}
	get := func(path string, status int, value any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer owner")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != status {
			t.Fatalf("%s: got %d, want %d: %s", path, resp.Code, status, resp.Body)
		}
		if value != nil {
			if err := json.Unmarshal(resp.Body.Bytes(), value); err != nil {
				t.Fatal(err)
			}
		}
	}
	alphaQuery := fmt.Sprintf("project_id=%d", alpha.ID)
	betaQuery := fmt.Sprintf("project_id=%d", beta.ID)
	var tasks store.TaskPage
	get("/tasks?"+alphaQuery+"&limit=1", 200, &tasks)
	if len(tasks.Items) != 1 || tasks.Items[0].ID != issue.ID || tasks.NextBefore != issue.ID ||
		tasks.Counts["ALL"] != 2 || tasks.Counts["RUNNING"] != 1 || tasks.Counts["WAITING_ACCEPTANCE"] != 1 {
		t.Fatal(tasks)
	}
	nextTasks := fmt.Sprintf("/tasks?%s&before=%d", alphaQuery, tasks.NextBefore)
	tasks = store.TaskPage{}
	get(nextTasks, 200, &tasks)
	if len(tasks.Items) != 1 || tasks.Items[0].ID != head.ID || tasks.NextBefore != 0 {
		t.Fatal(tasks)
	}
	get("/tasks?project_id=0", 400, nil)
	get("/tasks?project_id=%ZZ", 400, nil)
	get("/tasks?"+alphaQuery+"&"+betaQuery, 400, nil)
	get(fmt.Sprintf("/tasks/%d?%s", head.ID, alphaQuery), 200, nil)
	get(fmt.Sprintf("/tasks/%d?%s", head.ID, betaQuery), 404, nil)

	for _, endpoint := range []string{"/tasks/approvals", "/tasks/notifications", "/tasks/decision-questions"} {
		var page struct {
			Items []struct {
				ProjectID int64 `json:"project_id"`
			}
			Next json.RawMessage `json:"next_before"`
		}
		wantCount, query, project := 1, betaQuery, beta.ID
		if endpoint == "/tasks/approvals" {
			query, project = alphaQuery, alpha.ID
		}
		get(endpoint+"?"+query, 200, &page)
		if len(page.Items) != wantCount || page.Items[0].ProjectID != project ||
			(len(page.Next) != 0 && string(page.Next) != "null") {
			t.Fatalf("%s returned mixed or paginated project data: %+v", endpoint, page)
		}
		get(endpoint+"?project_id=", 400, nil)
	}
	var unread map[string]int64
	get("/tasks/notifications/unread?"+alphaQuery, 200, &unread)
	if unread["unread"] != 0 {
		t.Fatal(unread)
	}
	get("/tasks/notifications/unread?"+betaQuery, 200, &unread)
	if unread["unread"] != 1 {
		t.Fatal(unread)
	}
	var reviews store.ProjectCodeReviewPage
	get(fmt.Sprintf("/projects/%d/code-reviews", alpha.ID), 200, &reviews)
	if len(reviews.Items) != 0 {
		t.Fatal(reviews)
	}
	path := fmt.Sprintf("/projects/%d/code-reviews", beta.ID)
	get(path, 200, &reviews)
	if len(reviews.Items) != 1 || reviews.Items[0].State != "pending" || reviews.Items[0].Commit != commit || reviews.Items[0].Title != "Pending question" {
		t.Fatal(reviews)
	}
	get(fmt.Sprintf("%s?before=%d", path, reviews.Items[0].ExecutionID), 200, &reviews)
	if len(reviews.Items) != 0 {
		t.Fatal(reviews)
	}

	var projectPage struct{ Items []store.ProjectWorkspace }
	get("/projects?q=ACME%2FEXAMPLE", 200, &projectPage)
	want := store.ProjectMetrics{Running: 1, PendingApprovals: 1, AwaitingAcceptance: 1}
	if len(projectPage.Items) != 1 || projectPage.Items[0].ID != alpha.ID || projectPage.Items[0].CheckID != head.ID ||
		projectPage.Items[0].Repository != "acme/example" || projectPage.Items[0].Metrics != want {
		t.Fatal(projectPage)
	}
	var workspace store.ProjectWorkspace
	get(fmt.Sprintf("/projects/%d", beta.ID), 200, &workspace)
	if workspace.ID != beta.ID || workspace.Repository != "" || workspace.Metrics != (store.ProjectMetrics{PendingApprovals: 31}) {
		t.Fatal(workspace)
	}
	get(fmt.Sprintf("/projects/%d", beta.ID+1), 404, nil)
}
