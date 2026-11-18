package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIssueOutcomeFeedbackLease(t *testing.T) {
	db := isolatedDatabase(t)
	ctx := context.Background()
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 12, Number: 3, RepositoryID: 7}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.Claim(ctx, "writer", t.TempDir(), true)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	result := Result{ExitCode: 7, Error: "private error"}
	for range 2 {
		if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
			t.Fatal(err)
		}
	}
	if other, err := s.ClaimIssueOutcome(ctx, p.ID+1); err != nil || other != nil {
		t.Fatal(other, err)
	}
	v, err := s.ClaimIssueOutcome(ctx, p.ID)
	if err != nil || v == nil || v.Execution != job.ExecutionID || v.ExitCode == nil || *v.ExitCode != 7 || v.Kind != "execution_failed" {
		t.Fatal(v, err)
	}
	if next, err := s.ClaimIssueOutcome(ctx, p.ID); err != nil || next != nil {
		t.Fatal(next, err)
	}
	if _, err := db.Exec("UPDATE issue_outcome_feedback SET retry_at=now() WHERE event_id=$1", v.Event); err != nil {
		t.Fatal(err)
	}
	retry, err := s.ClaimIssueOutcome(ctx, p.ID)
	if err != nil || retry == nil || retry.Event != v.Event || retry.Lease == v.Lease {
		t.Fatal(retry, err)
	}
	if err := s.ConfirmIssueOutcome(ctx, *v); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.ConfirmIssueOutcome(ctx, *retry); err != nil {
		t.Fatal(err)
	}
	if next, err := s.ClaimIssueOutcome(ctx, p.ID); err != nil || next != nil {
		t.Fatal(next, err)
	}
	got, err := s.Task(ctx, task.ID)
	if err != nil || got.Status != "FAILED" || got.Events[len(got.Events)-1].IssueFeedbackAt == nil {
		t.Fatal("missing confirmation on original event", err)
	}
}
