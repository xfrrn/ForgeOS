package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReviewAndRework(t *testing.T) {
	db := isolatedDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	firstCommit := strings.Repeat("b", 40)
	secondCommit := strings.Repeat("c", 40)
	spec := IssueSpec{Issue: Issue{ID: 77, Number: 4, RepositoryID: 6}, BaseCommit: base, Validation: "docs"}
	issue, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	claim := func() *Job {
		t.Helper()
		job, err := s.Claim(ctx, "worker", t.TempDir(), true, true)
		if err != nil || job == nil {
			t.Fatalf("claim: %+v %v", job, err)
		}
		return job
	}
	complete := func(j *Job, sha string) {
		t.Helper()
		result := Result{Commit: sha, SessionID: strconv.FormatInt(j.ExecutionID, 10), BaseCommit: base, Validation: &ValidationResult{Profile: "docs", Commit: sha}}
		if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); err != nil {
			t.Fatal(err)
		}
	}
	first := claim()
	complete(first, firstCommit)
	classify := func(job *Job, sha, key, category string) int64 {
		t.Helper()
		raw, err := s.OpenPreview(ctx, issue.ID, job.ExecutionID, sha, "public", 8091, 60)
		var preview struct{ ID string }
		if err != nil || json.Unmarshal(raw, &preview) != nil {
			t.Fatalf("preview: %v %s", err, raw)
		}
		f := PreviewFeedback{Request: key, Execution: job.ExecutionID, Commit: sha, Path: "/", Note: "Checked the heading."}
		id, err := s.SubmitPreviewFeedback(ctx, issue.ID, preview.ID, f)
		if err != nil {
			t.Fatal(err)
		}
		j, err := s.ClaimFeedback(ctx, p.ID, true)
		if err != nil || j == nil || j.ID != id {
			t.Fatalf("claim feedback %d: %v %v", id, j, err)
		}
		if err = s.ClassifyFeedback(ctx, p.ID, id, j.Lease, FeedbackClassification{Category: category, Rationale: f.Note, Evidence: f.Note, Session: key}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	bug := classify(first, firstCommit, "13d375cf-a0e1-4539-b20a-032643e419a1", "bug")
	ui := classify(first, firstCommit, "13d375cf-a0e1-4539-b20a-032643e419a2", "ui_change")
	for _, refs := range [][]int64{{bug, ui, -1}, {ui, ui}} {
		if _, err := s.Review(ctx, issue.ID, first.ExecutionID, firstCommit, "reject", "revise", false, "", refs...); !errors.Is(err, ErrConflict) {
			t.Fatalf("accepted invalid references %v: %v", refs, err)
		}
	}
	if _, err := s.Review(ctx, issue.ID, first.ExecutionID, firstCommit, "approve", "", false, "", bug); !errors.Is(err, ErrConflict) {
		t.Fatalf("approved bug feedback: %v", err)
	}
	if _, err := s.Review(ctx, issue.ID, first.ExecutionID, secondCommit, "approve", "", true, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted stale SHA: %v", err)
	}
	if _, err := s.Review(ctx, issue.ID, first.ExecutionID, firstCommit, "reject", "", true, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted empty feedback: %v", err)
	}
	const feedback = "Correct the index as well."
	responses := make(chan error, 6)
	for i := 0; i < cap(responses); i++ {
		go func() {
			_, err := s.Review(ctx, issue.ID, first.ExecutionID, firstCommit, "reject", feedback, true, "main", ui, bug)
			responses <- err
		}()
	}
	for i := 0; i < cap(responses); i++ {
		if err := <-responses; err != nil {
			t.Fatal(err)
		}
	}
	rejected, err := s.Task(ctx, issue.ID)
	if err != nil || rejected.Status != "READY" || len(rejected.Reviews) != 1 || rejected.Reviews[0].Actor != "owner" ||
		rejected.Reviews[0].At.IsZero() || rejected.Delivery != nil || !reflect.DeepEqual(rejected.Reviews[0].FeedbackIDs, []int64{bug, ui}) || len(rejected.Events) != 8 {
		t.Fatalf("reject audit: %+v %v", rejected, err)
	}
	if _, err := s.Review(ctx, issue.ID, first.ExecutionID, firstCommit, "reject", feedback, true, "main", bug, ui); err != nil {
		t.Fatalf("reordered references: %v", err)
	}
	if _, err := s.Review(ctx, issue.ID, first.ExecutionID, firstCommit, "reject", feedback, true, "main", bug); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed decision references: %v", err)
	}
	if _, err := s.Review(ctx, issue.ID, first.ExecutionID, firstCommit, "approve", "", true, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted opposite decision: %v", err)
	}
	if j, err := s.Claim(ctx, "v2-worker", t.TempDir(), true); err != nil || j != nil {
		t.Fatalf("old worker claimed a rejected task: %v, %v", j != nil, err)
	}
	other := spec
	other.Issue.ID++
	other.Issue.Number++
	ordinary, err := s.ImportIssue(ctx, p.ID, other)
	if err != nil {
		t.Fatal(err)
	}
	legacyJob, err := s.Claim(ctx, "v2-worker", t.TempDir(), true)
	if err != nil || legacyJob == nil || legacyJob.TaskID != ordinary.ID {
		t.Fatalf("old worker could not bypass rework: %v", err)
	}
	complete(legacyJob, firstCommit)
	second := claim()
	if second.Rework == nil || second.Rework.Commit != firstCommit || second.Rework.Note != feedback || *second.Spec != spec {
		t.Fatalf("revision lost frozen input: %+v", second.Rework)
	}
	if err := s.Complete(ctx, second.ExecutionID, second.LeaseToken, Result{ExitCode: 1, Error: "disconnected"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetryIssueExecution(ctx, second.ExecutionID); err != nil {
		t.Fatal(err)
	}
	third := claim()
	if third.Rework == nil || !reflect.DeepEqual(third.Rework, second.Rework) {
		t.Fatal("retry lost the rejected version or feedback")
	}
	for _, result := range []Result{
		{Commit: firstCommit, SessionID: "fresh", BaseCommit: base, Validation: &ValidationResult{Profile: "docs", Commit: firstCommit}},
		{Commit: secondCommit, SessionID: strconv.FormatInt(first.ExecutionID, 10), BaseCommit: base, Validation: &ValidationResult{Profile: "docs", Commit: secondCommit}},
	} {
		if err := s.Complete(ctx, third.ExecutionID, third.LeaseToken, result); !errors.Is(err, ErrConflict) {
			t.Fatalf("reused rejected result accepted: %v", err)
		}
	}
	complete(third, secondCommit)
	if _, err := s.Review(ctx, issue.ID, third.ExecutionID, secondCommit, "reject", feedback, false, "", bug); !errors.Is(err, ErrConflict) {
		t.Fatalf("reused old report: %v", err)
	}
	passed := classify(third, secondCommit, "13d375cf-a0e1-4539-b20a-032643e419a3", "acceptance")
	local, err := s.Review(ctx, issue.ID, third.ExecutionID, secondCommit, "approve", "checked", false, "", passed)
	if err != nil || local.Status != "ACCEPTED" || local.Delivery != nil || len(local.Reviews) != 2 {
		t.Fatalf("local decision: %+v %v", local, err)
	}
	replay, err := s.Review(ctx, issue.ID, third.ExecutionID, secondCommit, "approve", "checked", true, "main", passed)
	if err != nil || replay.Delivery != nil || len(replay.Events) != len(local.Events) {
		t.Fatalf("replay authorized publication: %+v %v", replay, err)
	}
	head, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	j := claim()
	if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{Commit: secondCommit}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(ctx, head.ID, j.ExecutionID, secondCommit, "approve", "", true, ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("git_head delivery accepted: %v", err)
	}
}

func TestMigrateAndResumeDelivery(t *testing.T) {
	db := isolatedDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Existing V2 tasks must remain reviewable after a control-plane upgrade.
	if _, err := db.ExecContext(ctx, initialSchema+issueSchema+`
		CREATE TABLE forgeos_schema_versions (version integer PRIMARY KEY);
		INSERT INTO forgeos_schema_versions VALUES (1),(2);
		INSERT INTO projects(path, github_repository_id) VALUES ('previous-project',12);`); err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	sha := strings.Repeat("d", 40)
	for i := int64(1); i <= 2; i++ {
		spec, _ := json.Marshal(IssueSpec{Issue: Issue{ID: i, Number: i, RepositoryID: 12}, BaseCommit: base, Validation: "docs"})
		res, _ := json.Marshal(Result{Commit: sha, SessionID: "v2", BaseCommit: base, Validation: &ValidationResult{Profile: "docs", Commit: sha}})
		if _, err := db.ExecContext(ctx, `INSERT INTO work_items(project_id,kind,github_issue_id,status,spec)
			VALUES(1,'codex_issue',$1,'WAITING_ACCEPTANCE',$2)`, i, string(spec)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until,result)
			VALUES($1,'v2','path','token',now(),$2)`, i, string(res)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES($1,$1,'execution_completed')`, i); err != nil {
			t.Fatal(err)
		}
	}
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 2; i++ {
		approved, err := s.Review(ctx, i, i, sha, "approve", "", true, "main")
		if err != nil || approved.Status != "ACCEPTED" || approved.Delivery == nil || approved.Delivery.Commit != sha {
			t.Fatalf("upgrade lost approval input: %+v %v", approved, err)
		}
	}
	if wrong, err := s.ClaimDelivery(ctx, 2); err != nil || wrong != nil {
		t.Fatalf("project boundary: %v, %v", wrong != nil, err)
	}
	claim := func() Delivery {
		t.Helper()
		d, err := s.ClaimDelivery(ctx, 1)
		if err != nil || d == nil || d.LeaseToken == "" {
			t.Fatalf("delivery claim failed: %v", err)
		}
		return *d
	}
	first := claim()
	second := claim()
	if first.TaskID != 1 || second.TaskID != 2 {
		t.Fatal("active claim blocked another approved task")
	}
	if d, err := s.ClaimDelivery(ctx, 1); err != nil || d != nil {
		t.Fatalf("double claim: %v %v", d != nil, err)
	}
	expire := func(taskID int64) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `UPDATE deliveries SET available_at=now()-interval '1 second' WHERE task_id=$1`, taskID); err != nil {
			t.Fatal(err)
		}
	}
	expire(first.TaskID)
	next := claim()
	if err := s.SaveDelivery(ctx, first); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale lease overwrote new holder: %v", err)
	}
	next.PRNumber, next.PRURL, next.State = 7, "https://github.com/org/repo/pull/7", "WAITING"
	if err := s.SaveDelivery(ctx, next); err != nil {
		t.Fatal(err)
	}
	gate := claim()
	gate.Detail = "required status check pending"
	if err := s.SaveDelivery(ctx, gate); err != nil {
		t.Fatal(err)
	}
	if hits, err := s.SearchHistory(ctx, 1, first.TaskID, "merge", ""); err != nil || len(hits) != 0 {
		t.Fatalf("waiting PR entered merged project history: %v %v", hits, err)
	}
	if job, err := s.ClaimCuration(ctx, 1, false, false); err != nil || job != nil {
		t.Fatal("waiting PR queued", job, err)
	}
	if d, err := s.ClaimDelivery(ctx, 1); err != nil || d != nil {
		t.Fatalf("waiting check had no backoff: %v %v", d != nil, err)
	}
	expire(first.TaskID)
	persisted := claim()
	if persisted.PRNumber != 7 || persisted.Detail != gate.Detail {
		t.Fatal("lost remote checkpoint")
	}
	for _, mutate := range []func(*Delivery){
		func(d *Delivery) { d.Commit = base },
		func(d *Delivery) { d.TargetBranch = "elsewhere" },
		func(d *Delivery) { d.PRNumber = 8 },
		func(d *Delivery) { d.State = "PENDING" },
		func(d *Delivery) { d.State = "DONE" },
	} {
		bad := persisted
		mutate(&bad)
		if err := s.SaveDelivery(ctx, bad); !errors.Is(err, ErrConflict) {
			t.Fatalf("invalid delivery transition accepted: %v", err)
		}
	}
	persisted.State, persisted.MergeSHA = "MERGED", strings.Repeat("e", 40)
	if err := s.SaveDelivery(ctx, persisted); err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchHistory(ctx, 1, first.TaskID, "merge", persisted.MergeSHA)
	var receipt Delivery
	if err != nil || len(hits) != 1 || json.Unmarshal(hits[0].Delivery, &receipt) != nil ||
		hits[0].URL != persisted.PRURL || hits[0].SourceID != persisted.PRNumber || receipt.ExecutionID != first.ExecutionID ||
		receipt.Commit != sha || receipt.MergeSHA != persisted.MergeSHA {
		t.Fatalf("missing recorded PR merge: %v %v", hits, err)
	}
	merged := claim()
	job, err := s.ClaimCuration(ctx, 1, false, false)
	if err != nil || job == nil || !strings.Contains(string(job.Material), persisted.MergeSHA) {
		t.Fatal(job, err)
	}
	before, err := s.EventCuration(ctx, 1, first.TaskID, hits[0].Version)
	if err != nil || !strings.Contains(string(before), persisted.PRURL) {
		t.Fatal(string(before), err)
	}
	if _, err := s.EventCuration(ctx, 2, first.TaskID, hits[0].Version); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign merge", err)
	}
	bad := merged
	bad.MergeSHA = sha
	if err := s.SaveDelivery(ctx, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed merge SHA: %v", err)
	}
	merged.State, merged.Detail = "DONE", "issue closed"
	if err := s.SaveDelivery(ctx, merged); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDelivery(ctx, merged); !errors.Is(err, ErrConflict) {
		t.Fatalf("consumed lease accepted: %v", err)
	}
	if after, err := s.SearchHistory(ctx, 1, first.TaskID, "merge", sha); err != nil || !reflect.DeepEqual(hits, after) {
		t.Fatalf("final delivery or replay changed the merge receipt: %v %v", after, err)
	}
	if after, err := s.EventCuration(ctx, 1, first.TaskID, hits[0].Version); err != nil || string(after) != string(before) {
		t.Fatal("changed after delivery", err)
	}
	if next, err := s.ClaimCuration(ctx, 1, false, false); err != nil || next != nil {
		t.Fatal("duplicate merge", next, err)
	}
	final, err := New(db).Task(ctx, first.TaskID)
	if err != nil || final.Status != "DONE" || final.Delivery.LeaseToken != "" || final.Delivery.MergeSHA != merged.MergeSHA ||
		final.Delivery.State != "DONE" || len(final.Events) != 5 || final.Events[4].Type != "delivery_completed" {
		t.Fatalf("persisted final result: %+v %v", final, err)
	}
	page, err := s.ListTasks(ctx, TaskListFilter{Status: "DONE"})
	if err != nil || len(page.Items) != 1 || page.Items[0].PRNumber != merged.PRNumber ||
		page.Items[0].UpdatedAt == nil || !page.Items[0].UpdatedAt.Equal(final.Events[4].At) {
		t.Fatalf("completed delivery summary: %+v %v", page, err)
	}
}
