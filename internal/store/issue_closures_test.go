package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSourceClosureKeepsSlotUntilReceipt(t *testing.T) {
	s := New(isolatedDatabase(t))
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	observed := time.Now().UTC().Truncate(time.Second)
	spec := IssueSpec{Issue: Issue{ID: 7, Number: 1, RepositoryID: 8, URL: "https://github.com/acme/project/issues/1", UpdatedAt: observed.Format(time.RFC3339)}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	task, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	issue := TrackedIssue{ID: 7, Number: 1}
	closeIssue := func(stamp time.Time) {
		t.Helper()
		if err := s.CloseSourceIssue(ctx, p.ID, 8, issue, spec.Issue.URL, "not_planned", stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	closeIssue(observed.Add(-time.Second))
	tracked, err := s.TrackedIssues(ctx, p.ID, 8)
	if err != nil || len(tracked) != 1 || tracked[0] != issue {
		t.Fatal(tracked, err)
	}
	root := t.TempDir()
	job, err := s.Claim(ctx, "worker", root, true)
	if err != nil || job == nil || job.TaskID != task.ID {
		t.Fatal("claim failed", err)
	}
	nextTask, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	closeIssue(observed)
	closeIssue(observed)
	pending, err := s.Task(ctx, task.ID)
	if err != nil || pending.Status != "RUNNING" || pending.SourceClosure == nil || pending.Events[len(pending.Events)-1].Type != "source_cancel_requested" {
		t.Fatal("missing cancellation request", err)
	}
	if next, err := s.Claim(ctx, "another", root, true); err != nil || next != nil {
		t.Fatal("released project before receipt", err)
	}
	if err := s.RenewExecutionLease(ctx, job.ExecutionID, "wrong"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.RenewExecutionLease(ctx, job.ExecutionID, job.LeaseToken); !errors.Is(err, ErrSourceClosed) {
		t.Fatal(err)
	}
	result := Result{ExitCode: 1, Error: "cancelled"}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
		t.Fatal(err)
	}
	spec.Issue.UpdatedAt = observed.Add(time.Minute).Format(time.RFC3339)
	again, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil || again.ID != task.ID || again.Status != "CANCELLED" || again.Events[len(again.Events)-1].Type != "source_issue_cancelled" {
		t.Fatal("cancellation lost on reopen", err)
	}
	next, err := s.Claim(ctx, "another", root, true)
	if err != nil || next == nil || next.TaskID != nextTask.ID {
		t.Fatal("next task blocked after receipt", err)
	}
}

func TestClosedGoalPreservesApprovedChildAndArchivesUnfinishedChildren(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	p, err := s.CreateProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	spec := IssueSpec{Issue: Issue{ID: 101, Number: 1, RepositoryID: 42, URL: "https://github.com/acme/test/issues/1"}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	goal, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	item := PlanItem{Key: "first", Objective: "First", Scope: "docs", Acceptance: "Document exists", Deliverables: "README"}
	other, blocked := item, item
	other.Key = "second"
	blocked.Key, blocked.Dependencies = "third", []string{"second"}
	plan := Plan{Objective: "Documentation", Milestones: []Milestone{{Name: "Guide", Items: []PlanItem{item, other, blocked}}}}
	if _, err := s.SavePlan(ctx, goal.ID, 0, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivatePlan(ctx, goal.ID, 1); err != nil {
		t.Fatal(err)
	}
	claim := func() *Job {
		t.Helper()
		j, err := s.Claim(ctx, "worker", root, true)
		if err != nil || j == nil {
			t.Fatal("missing child claim", err)
		}
		return j
	}
	first := claim()
	commit := strings.Repeat("b", 40)
	result := Result{Commit: commit, BaseCommit: spec.BaseCommit, SessionID: "first", Validation: &ValidationResult{Commit: commit, Profile: "docs"}}
	if err := s.Complete(ctx, first.ExecutionID, first.LeaseToken, result); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(ctx, first.TaskID, first.ExecutionID, commit, "approve", "reviewed", false, ""); err != nil {
		t.Fatal(err)
	}
	second := claim()
	stamp := time.Now().UTC()
	if err := s.CloseSourceIssue(ctx, p.ID, 42, TrackedIssue{ID: 101, Number: 1}, spec.Issue.URL, "completed", stamp, stamp); err != nil {
		t.Fatal(err)
	}
	// A worker that finished before receiving the stop can still report its original result.
	result.SessionID = "second"
	if err := s.Complete(ctx, second.ExecutionID, second.LeaseToken, result); err != nil {
		t.Fatal(err)
	}
	after, err := s.Task(ctx, goal.ID)
	if err != nil || after.Status != "CANCELLED" || after.Plan == nil || len(after.Children) != 3 {
		t.Fatal("missing goal history", err)
	}
	for _, child := range after.Children {
		want := "CANCELLED"
		if child.ID == first.TaskID {
			want = "ACCEPTED"
		}
		if child.Status != want {
			t.Fatalf("child %d: %s, want %s", child.ID, child.Status, want)
		}
	}
	if _, err := s.ActivatePlan(ctx, goal.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("reactivated cancelled goal", err)
	}
	if next, err := s.Claim(ctx, "worker", root, true); err != nil || next != nil {
		t.Fatal("claimed cancelled child", err)
	}
}
