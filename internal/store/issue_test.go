package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIssueMigrationAndDelivery(t *testing.T) {
	db := isolatedDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, initialSchema+`
		CREATE TABLE forgeos_schema_versions (version integer PRIMARY KEY);
		INSERT INTO forgeos_schema_versions VALUES (1);
		INSERT INTO projects(path) VALUES ('v1-repo');
		INSERT INTO work_items(project_id,status) VALUES (1,'DONE');
		INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until,result)
		    VALUES (1,'v1','old-workspace','old-hash',now(),'{"exit_code":0,"commit":"old"}');
		INSERT INTO task_events(task_id,type) VALUES (1,'task_created');`)
	if err != nil {
		t.Fatal(err)
	}
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.Task(ctx, 1)
	if err != nil || legacy.Kind != "git_head" || legacy.Spec != nil || legacy.Execution.Result.Commit != "old" {
		t.Fatalf("v1 task after migration: %+v %v", legacy, err)
	}
	legacyPage, err := s.ListTasks(ctx, TaskListFilter{Limit: 1})
	if err != nil || len(legacyPage.Items) != 1 || legacyPage.Items[0].ID != legacy.ID ||
		legacyPage.Items[0].Worker != "v1" || legacyPage.Items[0].UpdatedAt == nil || legacyPage.Items[0].Title == "" {
		t.Fatalf("v1 list after migration: %+v %v", legacyPage, err)
	}

	spec := IssueSpec{
		Issue:      Issue{ID: 12, RepositoryID: 90, Repository: "org/repo", Number: 7, Title: "Fix spelling", Body: "Original text"},
		BaseCommit: strings.Repeat("0", 40), Validation: "docs",
	}
	task, err := s.ImportIssue(ctx, 1, spec)
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.Project(ctx, 1)
	if err != nil || project.RepositoryID != spec.Issue.RepositoryID {
		t.Fatalf("missing repository binding: %+v %v", project, err)
	}
	revised := spec
	revised.BaseCommit, revised.Issue.Body = strings.Repeat("1", 40), "edited"
	replays := make(chan Task, 4)
	for i := 0; i < cap(replays); i++ {
		go func() {
			task, err := s.ImportIssue(ctx, 1, revised)
			if err != nil {
				t.Error(err)
			}
			replays <- task
		}()
	}
	for i := 0; i < cap(replays); i++ {
		got := <-replays
		if got.ID != task.ID || got.Spec == nil || *got.Spec != spec || len(got.Events) != 1 {
			t.Fatalf("replay changed the imported snapshot: %+v", got)
		}
	}
	revised.Issue.RepositoryID++
	if _, err := s.ImportIssue(ctx, 1, revised); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted changed repository: %v", err)
	}
	root := t.TempDir()
	if next, err := s.Claim(ctx, "v1-worker", root); err != nil || next != nil {
		t.Fatalf("legacy worker claimed issue: %v %v", next != nil, err)
	}
	head, err := s.CreateTask(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.Claim(ctx, "v1-worker", root)
	if err != nil || h == nil || h.TaskID != head.ID || h.Spec != nil || h.Kind != "git_head" {
		t.Fatalf("legacy worker cannot bypass unsupported issue: %v", err)
	}
	if err := s.Complete(ctx, h.ExecutionID, h.LeaseToken, Result{Commit: spec.BaseCommit}); err != nil {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx, "codex-worker", root, true)
	if err != nil || j == nil || j.TaskID != task.ID || j.Kind != "codex_issue" || j.Spec == nil || *j.Spec != spec {
		t.Fatalf("issue claim lost source spec: %v", err)
	}
	got, err := s.Task(ctx, task.ID)
	if err != nil || got.Execution == nil || time.Until(got.Execution.LeaseUntil) < 20*time.Minute {
		t.Fatalf("issue lease too short: %+v %v", got, err)
	}
	candidate := strings.Repeat("a", 40)
	check := ValidationResult{Commit: candidate, Profile: "docs", Output: "pass"}
	success := Result{Commit: candidate, BaseCommit: spec.BaseCommit, SessionID: "thread-1", Validation: &check}
	for _, mutate := range []func(*Result){
		func(r *Result) { r.SessionID = "" },
		func(r *Result) { r.Commit = spec.BaseCommit; r.Validation.Commit = r.Commit },
		func(r *Result) { r.Validation.Commit = spec.BaseCommit },
		func(r *Result) { r.Validation.Profile = "go-test" },
		func(r *Result) { r.Validation.ExitCode = 1 },
		func(r *Result) { r.Validation = nil },
	} {
		bad, v := success, check
		bad.Validation = &v
		mutate(&bad)
		if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, bad); !errors.Is(err, ErrConflict) {
			t.Fatalf("accepted contradictory issue report: %+v %v", bad, err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, success); err != nil {
			t.Fatal(err)
		}
	}
	final, err := s.Task(ctx, task.ID)
	if err != nil || final.Status != "WAITING_ACCEPTANCE" || final.Execution.Status != "COMPLETED" ||
		!reflect.DeepEqual(final.Execution.Result, &success) || len(final.Events) != 3 {
		t.Fatalf("lost candidate or validation: %+v %v", final, err)
	}

	spec.Issue.ID++
	spec.Issue.Number++
	next, err := s.ImportIssue(ctx, 1, spec)
	if err != nil {
		t.Fatal(err)
	}
	j, err = s.Claim(ctx, "codex-worker", root, true)
	if err != nil || j == nil || j.TaskID != next.ID {
		t.Fatalf("cannot claim next issue: %v", err)
	}
	check.ExitCode, check.Output = 1, "check failed"
	failure := Result{Commit: candidate, BaseCommit: spec.BaseCommit, SessionID: "thread-2", Validation: &check, ExitCode: 1, Error: "validation failed"}
	if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, failure); err != nil {
		t.Fatal(err)
	}
	failed, err := s.Task(ctx, next.ID)
	if err != nil || failed.Status != "FAILED" || !reflect.DeepEqual(failed.Execution.Result, &failure) {
		t.Fatalf("lost failed validation: %+v %v", failed, err)
	}
}

func TestRetryIssueExecution(t *testing.T) {
	db := isolatedDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := IssueSpec{Issue: Issue{ID: 91, RepositoryID: 5, Number: 3}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	item, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	claim := func() *Job {
		t.Helper()
		j, err := s.Claim(ctx, "worker", root, true)
		if err != nil || j == nil || j.TaskID != item.ID || j.Spec == nil || *j.Spec != spec {
			t.Fatalf("expected attempt for the same frozen task: %v", err)
		}
		return j
	}
	first := claim()
	if _, err := s.RetryIssueExecution(ctx, first.ExecutionID); !errors.Is(err, ErrConflict) {
		t.Fatalf("running execution retried: %v", err)
	}
	failure := Result{ExitCode: 1, Error: "Codex unavailable"}
	if err := s.Complete(ctx, first.ExecutionID, first.LeaseToken, failure); err != nil {
		t.Fatal(err)
	}
	requests := make(chan error, 6)
	for i := 0; i < cap(requests); i++ {
		go func() {
			_, err := s.RetryIssueExecution(ctx, first.ExecutionID)
			requests <- err
		}()
	}
	for i := 0; i < cap(requests); i++ {
		if err := <-requests; err != nil {
			t.Fatal(err)
		}
	}
	queued, err := s.Task(ctx, item.ID)
	if err != nil || queued.Status != "READY" || len(queued.Events) != 4 || queued.Events[3].Type != "execution_retry_requested" {
		t.Fatalf("duplicate retries: %+v %v", queued, err)
	}
	second := claim()
	if second.ExecutionID == first.ExecutionID || second.Workspace == first.Workspace {
		t.Fatal("retry reused old attempt or workspace")
	}
	if err := s.Complete(ctx, first.ExecutionID, first.LeaseToken, failure); err != nil {
		t.Fatalf("old duplicate report: %v", err)
	}
	latest, err := s.RetryIssueExecution(ctx, first.ExecutionID)
	if err != nil || latest.Status != "RUNNING" || len(latest.Attempts) != 2 || latest.Attempts[0].Status != "FAILED" || latest.Attempts[1].Status != "RUNNING" {
		t.Fatalf("old request affected running attempt: %+v %v", latest, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE executions SET lease_until = now() - interval '1 second' WHERE id = $1`, second.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if err := s.Abandon(ctx, second.ExecutionID); err != nil {
		t.Fatal(err)
	}
	stale, err := s.RetryIssueExecution(ctx, first.ExecutionID)
	if err != nil || stale.Status != "FAILED" {
		t.Fatalf("old request retried newer failure: %+v %v", stale, err)
	}
	if _, err := s.RetryIssueExecution(ctx, second.ExecutionID); err != nil {
		t.Fatal(err)
	}
	third := claim()
	sha := strings.Repeat("c", 40)
	ok := Result{BaseCommit: spec.BaseCommit, Commit: sha, SessionID: "thread-3", Validation: &ValidationResult{Profile: "docs", Commit: sha}}
	if err := s.Complete(ctx, first.ExecutionID, first.LeaseToken, ok); !errors.Is(err, ErrConflict) {
		t.Fatalf("replaced first attempt's actual result: %v", err)
	}
	if err := s.Complete(ctx, second.ExecutionID, second.LeaseToken, ok); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted late report for abandoned attempt: %v", err)
	}
	if err := s.Complete(ctx, third.ExecutionID, third.LeaseToken, ok); err != nil {
		t.Fatal(err)
	}
	final, err := s.RetryIssueExecution(ctx, second.ExecutionID)
	if err != nil || final.Status != "WAITING_ACCEPTANCE" || final.Execution.ID != third.ExecutionID || len(final.Attempts) != 3 ||
		final.Attempts[0].Status != "FAILED" || final.Attempts[1].Status != "FAILED" || final.Attempts[1].AbandonedAt == nil ||
		final.Attempts[2].Status != "COMPLETED" || len(final.Events) != 9 {
		t.Fatalf("lost attempt history: %+v %v", final, err)
	}
	if _, err := s.RetryIssueExecution(ctx, third.ExecutionID); !errors.Is(err, ErrConflict) {
		t.Fatalf("successful execution retried: %v", err)
	}
}
