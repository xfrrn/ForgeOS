package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/testdb"
)

func isolatedDatabase(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func TestClaimRechecksTaskAfterPlanningLock(t *testing.T) {
	db := isolatedDatabase(t)
	s := New(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 7, Number: 1, RepositoryID: 8}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var holder int
	if err := tx.QueryRowContext(ctx, `UPDATE work_items SET status='PLANNING' WHERE id=$1 RETURNING pg_backend_pid()`, task.ID).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	root := t.TempDir()
	go func() {
		j, err := s.Claim(ctx, "worker", root, true)
		if j != nil {
			err = errors.New("worker claimed an item already in planning")
		}
		done <- err
	}()
	for {
		var waiting bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, holder).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDispatchAndReplay(t *testing.T) {
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
	again, err := s.CreateProject(ctx, p.Path)
	if err != nil || again != p {
		t.Fatalf("repeat registration: %+v, %v", again, err)
	}
	if _, err := s.CreateTask(ctx, p.ID+10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unregistered project: %v", err)
	}
	first, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	type claim struct {
		job *Job
		err error
	}
	claims := make(chan claim, 8)
	for i := 0; i < cap(claims); i++ {
		go func() {
			j, err := s.Claim(ctx, "worker", root)
			claims <- claim{j, err}
		}()
	}
	var job *Job
	for i := 0; i < cap(claims); i++ {
		result := <-claims
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.job != nil {
			if job != nil {
				t.Fatal("two executions claimed in one project")
			}
			job = result.job
		}
	}
	if job == nil || job.TaskID != first.ID || filepath.Dir(job.Workspace) != root {
		t.Fatalf("unexpected claim: %+v", job != nil)
	}
	if _, err := db.ExecContext(ctx, `UPDATE executions SET lease_until = now() - interval '1 minute' WHERE id=$1`, job.ExecutionID); err != nil {
		t.Fatal(err)
	}
	expired, err := s.Task(ctx, first.ID)
	if err != nil || expired.Execution == nil || !expired.Execution.LeaseExpired || expired.Execution.Workspace != job.Workspace {
		t.Fatalf("lost-worker evidence: %+v %v", expired, err)
	}
	if next, err := s.Claim(ctx, "another-worker", root); err != nil || next != nil {
		t.Fatalf("expired lease allowed automatic dispatch: claimed=%t, %v", next != nil, err)
	}
	ok := Result{Commit: strings.Repeat("a", 40), ExitCode: 0}
	if err := s.Complete(ctx, job.ExecutionID, "wrong token", ok); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong lease: %v", err)
	}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, ok); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, ok); err != nil {
		t.Fatalf("duplicate report: %v", err)
	}
	if err := s.Abandon(ctx, job.ExecutionID); !errors.Is(err, ErrConflict) {
		t.Fatalf("owner overwrote actual result: %v", err)
	}
	failure := Result{ExitCode: 1, Error: "git command failed"}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, failure); !errors.Is(err, ErrConflict) {
		t.Fatalf("contradictory report: %v", err)
	}
	if err := New(db).Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	finished, err := New(db).Task(ctx, first.ID)
	if err != nil || finished.Status != "DONE" || finished.Execution == nil || finished.Execution.Result == nil || *finished.Execution.Result != ok || len(finished.Events) != 3 {
		t.Fatalf("persisted completion: %+v %v", finished, err)
	}
	next, err := s.Claim(ctx, "another-worker", root)
	if err != nil || next == nil || next.TaskID != second.ID {
		t.Fatalf("queued task not released: claimed=%t, %v", next != nil, err)
	}
	if err := s.Complete(ctx, next.ExecutionID, next.LeaseToken, failure); err != nil {
		t.Fatal(err)
	}
	failed, err := s.Task(ctx, second.ID)
	if err != nil || failed.Status != "FAILED" || failed.Execution == nil || failed.Execution.Result == nil || *failed.Execution.Result != failure {
		t.Fatalf("failed result: %+v %v", failed, err)
	}
}

func TestAbandonExpiredExecution(t *testing.T) {
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
	root := t.TempDir()
	start := func() *Job {
		t.Helper()
		if _, err := s.CreateTask(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		j, err := s.Claim(ctx, "worker", root)
		if err != nil || j == nil {
			t.Fatalf("expected a claim: %v", err)
		}
		return j
	}
	expire := func(j *Job) {
		t.Helper()
		if _, err := db.ExecContext(ctx, `UPDATE executions SET lease_until = now() - interval '1 second' WHERE id = $1`, j.ExecutionID); err != nil {
			t.Fatal(err)
		}
	}
	j := start()
	if err := s.Abandon(ctx, j.ExecutionID); !errors.Is(err, ErrConflict) {
		t.Fatalf("abandoned live lease: %v", err)
	}
	expire(j)
	for i := 0; i < 2; i++ {
		if err := s.Abandon(ctx, j.ExecutionID); err != nil {
			t.Fatal(err)
		}
	}
	result := Result{Commit: strings.Repeat("f", 40)}
	if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted report after abandonment: %v", err)
	}
	lost, err := s.Task(ctx, j.TaskID)
	if err != nil || lost.Status != "FAILED" || lost.Execution == nil || lost.Execution.AbandonedAt == nil || lost.Execution.Result != nil || len(lost.Events) != 3 || lost.Events[2].Type != "execution_abandoned" {
		t.Fatalf("missing lost-worker evidence: %+v %v", lost, err)
	}
	j = start()
	expire(j)
	done := make(chan error, 2)
	go func() { done <- s.Abandon(ctx, j.ExecutionID) }()
	go func() { done <- s.Complete(ctx, j.ExecutionID, j.LeaseToken, result) }()
	a, b := <-done, <-done
	if !((a == nil && errors.Is(b, ErrConflict)) || (b == nil && errors.Is(a, ErrConflict))) {
		t.Fatalf("concurrent owner/worker finalization: %v and %v", a, b)
	}
}
