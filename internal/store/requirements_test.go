package store

import (
	"context"
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
		if err := s.ObserveIssue(ctx, p.ID, issue); err != nil {
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
