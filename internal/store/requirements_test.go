package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRequirementHistoryIgnoresDelayedPolls(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "requirements")
	if err != nil {
		t.Fatal(err)
	}
	issue := Issue{ID: 4, Number: 1, RepositoryID: 31, Title: "Handbook", Body: "original", UpdatedAt: "2026-01-01T09:00:00Z"}
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: issue, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	observe := func(body, stamp string) {
		t.Helper()
		issue.Body, issue.UpdatedAt = body, stamp
		if _, err := s.ObserveIssue(ctx, p.ID, issue); err != nil {
			t.Fatal(err)
		}
	}
	observe("original", "2026-01-01T11:00:00Z")
	observe("stale", "2026-01-01T10:00:00Z")
	observe("new scope", "2026-01-01T12:00:00Z")
	observe("new scope", "2026-01-01T12:00:00Z")
	now, err := s.Task(ctx, task.ID)
	if err != nil || now.LatestRequirement != 2 || now.RequirementRevision != 1 || now.Spec.Issue.Body != "original" {
		t.Fatal(now, err)
	}
	observed, err := s.RequirementRevision(ctx, task.ID, 2)
	if err != nil || observed.Issue.Body != "new scope" || observed.ID <= 0 || observed.TaskID != task.ID || observed.ObservedAt.IsZero() {
		t.Fatal(observed, err)
	}
	item := PlanItem{Key: "x", Objective: "Example", Scope: "Docs", Acceptance: "Link resolves", Deliverables: "README"}
	plan := Plan{Objective: "Docs", Milestones: []Milestone{{Name: "Docs", Items: []PlanItem{item}}}}
	if _, err = s.SavePlan(ctx, task.ID, 0, plan); err != nil {
		t.Fatal(err)
	}
	goal, err := s.ActivatePlan(ctx, task.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	child := goal.Children[0].ID
	if _, err = s.Claim(ctx, "worker", t.TempDir(), true); err != nil {
		t.Fatal(err)
	}
	bound, err := s.Task(ctx, child)
	if err != nil || bound.RequirementRevision != 1 || bound.LatestRequirement != 2 || bound.Execution == nil || bound.Execution.RequirementRevision != 1 {
		t.Fatal(bound, err)
	}
	history, err := s.RequirementRevision(ctx, child, 2)
	if err != nil || history.ID != observed.ID {
		t.Fatal(history, err)
	}
}

func TestAdoptRequirementAfterTurn(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "adoption")
	if err != nil {
		t.Fatal(err)
	}
	spec := IssueSpec{Issue: Issue{ID: 7, Number: 1, RepositoryID: 42, Body: "original"}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	task, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	job, err := s.Claim(ctx, "worker", root, true)
	if err != nil || job == nil {
		t.Fatal(job, err)
	}
	issue := spec.Issue
	issue.Body = "clarification"
	if _, err = s.ObserveIssue(ctx, p.ID, issue); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdoptRequirement(ctx, task.ID, 2); !errors.Is(err, ErrConflict) {
		t.Fatal("changed running input", err)
	}
	candidate := strings.Repeat("b", 40)
	result := Result{Commit: candidate, BaseCommit: spec.BaseCommit, SessionID: "original", Validation: &ValidationResult{Commit: candidate, Profile: "docs"}}
	if err = s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Review(ctx, task.ID, job.ExecutionID, candidate, "reject", "old wording", false, ""); err != nil {
		t.Fatal(err)
	}
	issue.Body = "updated requirements"
	if _, err = s.ObserveIssue(ctx, p.ID, issue); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdoptRequirement(ctx, task.ID, 2); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted stale source", err)
	}
	for range 2 {
		updated, err := s.AdoptRequirement(ctx, task.ID, 3)
		if err != nil || updated.Status != "READY" || updated.RequirementRevision != 3 || updated.Execution.RequirementRevision != 1 {
			t.Fatal(updated, err)
		}
	}
	next, err := s.Claim(ctx, "worker", root, true, false)
	if err != nil || next == nil || next.Rework != nil || next.Spec.Issue.Body != issue.Body {
		t.Fatal("carried stale rejection", next, err)
	}
	updated, err := s.Task(ctx, task.ID)
	if err != nil || updated.Execution.RequirementRevision != 3 || len(updated.Attempts) != 2 || updated.Attempts[0].Result.Commit != candidate {
		t.Fatal(updated, err)
	}
}
