package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestProjectWorkViewsUseLiveProjectRecords(t *testing.T) {
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
	running, err := data.CreateTask(ctx, alpha.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.Claim(ctx, "current-runner", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ready, err := data.CreateTask(ctx, alpha.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until,result)
		VALUES ($1,'old-runner','/work','',now(),'{}')`, ready.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `WITH goal AS (
		INSERT INTO work_items(project_id,status,kind,github_issue_id,spec)
		VALUES ($1,'ACTIVE','codex_issue',100,'{"issue":{"title":"Alpha Goal"}}') RETURNING id
	), plan AS (
		INSERT INTO plan_revisions(task_id,revision,plan) SELECT id,1,'{}' FROM goal RETURNING task_id,revision
	), child AS (
		INSERT INTO work_items(project_id,status,kind,spec,goal_id,plan_revision,plan_key)
		SELECT $1,'BLOCKED','codex_issue','{"issue":{"title":"Alpha Dependent"}}',task_id,revision,'dependent' FROM plan RETURNING id
	)
	INSERT INTO work_dependencies(task_id,prerequisite) SELECT id,$2 FROM child`, alpha.ID, ready.ID); err != nil {
		t.Fatal(err)
	}
	addCandidate := func(project, issue int64, label string, passed bool) int64 {
		t.Helper()
		var execution int64
		if err := db.QueryRowContext(ctx, `WITH task AS (
			INSERT INTO work_items(project_id,status,kind,github_issue_id,spec)
			VALUES ($1,'WAITING_ACCEPTANCE','codex_issue',$2,jsonb_build_object('issue',jsonb_build_object('title',$3::text))) RETURNING id
		), candidate AS (
			INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until,result)
			SELECT id,'test-runner','/work','',now(),jsonb_build_object('commit',repeat('a',40),'validation',
				jsonb_build_object('commit',repeat('a',40),'profile','docs','exit_code',CASE WHEN $4::boolean THEN 0 ELSE 1 END))
			FROM task RETURNING id,task_id
		), plan AS (
			INSERT INTO candidate_test_plans(execution_id,task_id,revision,requested_criteria,source)
			SELECT id,task_id,1,'[]','{}' FROM candidate WHERE $4 RETURNING id
		), job AS (
			INSERT INTO candidate_test_plan_jobs(plan_id,role,lease,lease_until)
			SELECT id,'test',gen_random_uuid(),now()+interval '5 minutes' FROM plan
		)
		SELECT id FROM candidate`, project, issue, label, passed).Scan(&execution); err != nil {
			t.Fatal(err)
		}
		return execution
	}
	alphaExecution := addCandidate(alpha.ID, 101, "Alpha Candidate", true)
	addCandidate(beta.ID, 102, "Beta Candidate", false)
	mux := http.NewServeMux()
	registerProjectWorkViews(mux, data, "owner")
	read := func(project int64, path, auth string, status int, value any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/projects/%d/%s", project, path), nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != status {
			t.Fatalf("%s: got %d, want %d: %s", path, response.Code, status, response.Body)
		}
		if value != nil {
			if err := json.Unmarshal(response.Body.Bytes(), value); err != nil {
				t.Fatal(err)
			}
		}
	}
	var queue store.ProjectExecutionQueue
	read(alpha.ID, "execution-queue", "owner", http.StatusOK, &queue)
	if queue.Counts["RUNNING"] != 1 || queue.Counts["READY"] != 1 || queue.Counts["BLOCKED"] != 1 || len(queue.Items) != 3 ||
		queue.Items[0].Worker != "current-runner" || queue.Items[1].TaskID != ready.ID ||
		queue.Items[1].Worker != "" || queue.Items[1].HeldByTaskID != running.ID ||
		len(queue.Items[2].BlockedBy) != 1 || queue.Items[2].BlockedBy[0] != ready.ID {
		t.Fatalf("queue did not bind current execution correctly: %+v", queue)
	}
	if len(queue.Roles) != 1 || queue.Roles[0].Role != "test_plan" || queue.Roles[0].Running != 1 || queue.Roles[0].Pending != 0 {
		t.Fatalf("missing leased test agent job: %+v", queue.Roles)
	}
	queue = store.ProjectExecutionQueue{}
	read(beta.ID, "execution-queue", "owner", http.StatusOK, &queue)
	if len(queue.Items) != 0 || len(queue.Roles) != 0 {
		t.Fatalf("beta leaked alpha queue data: %+v", queue)
	}
	var checks store.ProjectTestCheckPage
	read(alpha.ID, "test-checks", "owner", http.StatusOK, &checks)
	if len(checks.Items) != 1 || checks.Items[0].ExecutionID != alphaExecution || checks.Items[0].CIState != "passed" ||
		checks.Items[0].PlanRevision != 1 || checks.Items[0].PlanState != "awaiting_plan" {
		t.Fatalf("unexpected test and CI receipts: %+v", checks)
	}
	checks = store.ProjectTestCheckPage{}
	read(beta.ID, "test-checks", "owner", http.StatusOK, &checks)
	if len(checks.Items) != 1 || checks.Items[0].Title != "Beta Candidate" || checks.Items[0].CIState != "failed" || checks.Items[0].PlanRevision != 0 {
		t.Fatalf("unexpected beta receipts: %+v", checks)
	}
	read(alpha.ID, fmt.Sprintf("test-checks?before=%d", alphaExecution), "owner", http.StatusOK, &checks)
	if len(checks.Items) != 0 {
		t.Fatalf("cursor did not exclude newer receipts: %+v", checks)
	}
	read(alpha.ID, "test-checks?before=0", "owner", http.StatusBadRequest, nil)
	read(alpha.ID, "execution-queue", "", http.StatusUnauthorized, nil)
	read(99999, "test-checks", "owner", http.StatusNotFound, nil)
}
