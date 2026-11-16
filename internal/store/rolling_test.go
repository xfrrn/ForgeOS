package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRollingDraftHoldsReadyWorkUntilApproval(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "drafts")
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 2, Number: 1, RepositoryID: 12}, BaseCommit: strings.Repeat("e", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	item := PlanItem{Key: "example", Objective: "Example", Scope: "README", Deliverables: "README", Acceptance: "Example matches CLI"}
	plan := Plan{Objective: "Usage", Milestones: []Milestone{{Name: "Docs", Items: []PlanItem{item}}}}
	if _, err = s.SavePlan(ctx, task.ID, 0, plan); err != nil {
		t.Fatal(err)
	}
	first, err := s.ActivatePlan(ctx, task.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	plan.Milestones[0].Items[0].Scope = "Guide"
	if _, err = s.SavePlan(ctx, task.ID, 1, plan); err != nil {
		t.Fatal(err)
	}
	if job, err := s.Claim(ctx, "worker", t.TempDir(), true); err != nil || job != nil {
		t.Fatal("claim during revision", job, err)
	}
	if _, err = s.ActivatePlan(ctx, task.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("stale activation", err)
	}
	active, err := s.ActivatePlan(ctx, task.ID, 2)
	if err != nil || active.ActiveRevision != 2 || active.Children[0].Revision != 2 || first.Children[0].Revision != 1 || active.Children[0].ID != first.Children[0].ID {
		t.Fatal(active, err)
	}
	job, err := s.Claim(ctx, "worker", t.TempDir(), true)
	if err != nil || job == nil || job.TaskID != first.Children[0].ID || !strings.Contains(job.Spec.Issue.Body, "Scope: Guide") {
		t.Fatal(job, err)
	}
}
