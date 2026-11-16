package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestUpgradeIssueToVersionedGoal(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 7, Number: 1, RepositoryID: 8},
		BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	first := PlanItem{Key: "a", Objective: "A", Scope: "docs", Acceptance: "Example exists", Deliverables: "README"}
	second := first
	second.Key, second.Dependencies = "b", []string{"a"}
	plan := Plan{Objective: "Document usage", Milestones: []Milestone{{"Handbook", []PlanItem{second, first}}}}
	root := t.TempDir()
	job, err := s.Claim(ctx, "worker", root, true)
	if err != nil || job == nil {
		t.Fatalf("claim: %v %v", job, err)
	}
	if _, err = s.SavePlan(ctx, task.ID, 0, plan); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed active execution: %v", err)
	}
	if err = s.Complete(ctx, job.ExecutionID, job.LeaseToken, Result{ExitCode: 1, Error: "requires planning"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if r, err := s.SavePlan(ctx, task.ID, 0, plan); err != nil || r.Revision != 1 {
			t.Fatalf("save/replay: %+v %v", r, err)
		}
	}
	plan.Objective = "Expand handbook"
	if _, err = s.SavePlan(ctx, task.ID, 0, plan); !errors.Is(err, ErrConflict) {
		t.Fatalf("overwrote plan: %v", err)
	}
	if _, err = s.SavePlan(ctx, task.ID, 1, plan); err != nil {
		t.Fatal(err)
	}
	got, err := s.Task(ctx, task.ID)
	if err != nil || got.Plan == nil || got.Plan.Revision != 2 || got.Status != "PLANNING" || len(got.Events) != 5 ||
		len(got.Attempts) != 1 || got.Execution.ID != job.ExecutionID {
		t.Fatalf("lost goal history: %+v %v", got, err)
	}
	old, err := s.PlanRevision(ctx, task.ID, 1)
	if err != nil || old.Plan.Objective != "Document usage" {
		t.Fatalf("lost original: %+v %v", old, err)
	}
	if next, err := s.Claim(ctx, "worker", root, true); next != nil || err != nil {
		t.Fatalf("dispatched draft: %v %v", next, err)
	}
	for _, dep := range []string{"b", "missing"} {
		plan.Milestones[0].Items[1].Dependencies = []string{dep}
		if _, err := s.SavePlan(ctx, task.ID, 2, plan); !errors.Is(err, ErrConflict) {
			t.Fatalf("accepted invalid graph %s: %v", dep, err)
		}
	}
}
