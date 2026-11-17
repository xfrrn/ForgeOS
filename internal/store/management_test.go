package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"forgeos/internal/validation"
)

func TestManagementBindsPolicyLeaseAndRevision(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "handbook")
	if err != nil {
		t.Fatal(err)
	}
	spec := IssueSpec{Issue: Issue{ID: 1, Number: 1, RepositoryID: 9}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	task, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	item := PlanItem{Key: "examples", Objective: "Document usage", Scope: "README", Deliverables: "README", Acceptance: "Examples match CLI"}
	plan := Plan{Objective: "Handbook", Milestones: []Milestone{{Name: "Documentation", Items: []PlanItem{item}}}}
	if _, err = s.SavePlan(ctx, task.ID, 0, plan); err != nil {
		t.Fatal(err)
	}
	claim := func() *ManagementJob {
		t.Helper()
		job, err := s.ClaimManagement(ctx, p.ID, true)
		if err != nil || job == nil || !job.CaptureProfile {
			t.Fatal(job, err)
		}
		return job
	}
	if job, err := s.ClaimManagement(ctx, p.ID+1, true); err != nil || job != nil {
		t.Fatal(job, err)
	}
	first := claim()
	pool := ModelBudget{Name: "plans", Harness: "codex", Slots: 1, AgentRoles: []ModelBudgetRole{{Project: p.ID, Role: "plan_review"}}}
	if err := s.SetModelBudgets(ctx, []ModelBudget{pool}); err != nil {
		t.Fatal(err)
	}
	used := func(want int) {
		t.Helper()
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT reserved FROM model_budget_usage WHERE name=$1`, pool.Name).Scan(&n); err != nil || n != want {
			t.Fatalf("reserved %d, want %d: %v", n, want, err)
		}
	}
	used(1)
	if job, err := s.ClaimManagement(ctx, p.ID, true); err != nil || job != nil {
		t.Fatal(job, err)
	}
	if err = s.SetPlanMinimum(ctx, task.ID, 1, "D1"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	profile := StructuredCodexProfile()
	review := ManagementReview{Verdict: "approve", Level: "D1", Rationale: "examples match issue", Evidence: "README", SessionID: "manager", Profile: &profile}
	if _, err = s.SavePlanWithPolicy(ctx, task.ID, 1, plan, "D3"); err != nil {
		t.Fatal(err)
	}
	if job, err := s.ClaimManagement(ctx, p.ID, true); err != nil || job != nil {
		t.Fatal("superseded review still running", job, err)
	}
	if err = s.CompleteManagement(ctx, p.ID, task.ID, 1, first.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal("stale accepted", err)
	}
	used(0)
	if job, err := s.ClaimManagement(ctx, p.ID, false); err != nil || job != nil {
		t.Fatal("legacy worker received budgeted plan", job, err)
	}
	second := claim()
	if second.LeaseUntil == nil {
		t.Fatal("missing plan lease deadline")
	}
	used(1)
	if _, err = s.db.ExecContext(ctx, `UPDATE plan_revisions SET review_until=now() WHERE task_id=$1 AND revision=2`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteManagement(ctx, p.ID, task.ID, 2, second.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal("expired accepted", err)
	}
	used(0)
	reissued := claim()
	if reissued.Lease == second.Lease {
		t.Fatal("lease reused")
	}
	if err = s.CompleteManagement(ctx, p.ID, task.ID, 2, second.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal("old lease accepted", err)
	}
	if err = s.CompleteManagement(ctx, p.ID+1, task.ID, 2, reissued.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal("cross project", err)
	}
	for range 2 {
		if err = s.CompleteManagement(ctx, p.ID, task.ID, 2, reissued.Lease, review); err != nil {
			t.Fatal(err)
		}
	}
	used(0)
	pending, err := s.Task(ctx, task.ID)
	if err != nil || pending.Status != "PLANNING" || len(pending.Children) != 0 || pending.Plan.Review.EffectiveLevel != "D3" ||
		pending.Plan.Review.Profile == nil || !validStructuredProfile(pending.Plan.Review.Profile) {
		t.Fatal(pending, err)
	}
	if _, err = s.SavePlan(ctx, task.ID, 2, plan); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPlanMinimum(ctx, task.ID, 3, "D1"); err != nil {
		t.Fatal(err)
	}
	last := claim()
	for range 2 {
		if err = s.CompleteManagement(ctx, p.ID, task.ID, 3, last.Lease, review); err != nil {
			t.Fatal(err)
		}
	}
	active, err := s.Task(ctx, task.ID)
	if err != nil || active.Status != "ACTIVE" || len(active.Children) != 1 || active.Plan.Review.EffectiveLevel != "D1" {
		t.Fatal(active, err)
	}
	review.Verdict = "revise"
	if err = s.CompleteManagement(ctx, p.ID, task.ID, 3, last.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay accepted", err)
	}
	spec.Issue.ID, spec.Issue.Number, spec.Validation = 2, 2, "go-test"
	code, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePlanWithPolicy(ctx, code.ID, 0, plan, "D1"); !errors.Is(err, ErrConflict) {
		t.Fatal("code allowed D1", err)
	}
}

func TestManagementValidationMinimumAppliesOnActivation(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, "queue")
	must(err)
	task, err := s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{ID: 1, Number: 1, RepositoryID: 9}, BaseCommit: strings.Repeat("a", 40), Validation: "auto", Policy: &validation.Policy{Target: "pull_request"}})
	must(err)
	item := PlanItem{Key: "release", Objective: "Verify queue", Scope: "queue", Deliverables: "report", Acceptance: "Capacity boundary holds"}
	second := item
	second.Key = "handoff"
	plan := Plan{Objective: "Release", Milestones: []Milestone{{Name: "Gate", Items: []PlanItem{item, second}}}}
	for i, floor := range []string{"V3", "V4", "V1"} {
		rev, err := s.SavePlan(ctx, task.ID, i, plan)
		must(err)
		job, err := s.ClaimManagement(ctx, project.ID, true)
		must(err)
		if job == nil {
			t.Fatal("no review job")
		}
		review := ManagementReview{Verdict: "approve", Level: "D2", Rationale: "release gate", Evidence: "queue test plan", SessionID: "manager", ValidationMinimum: floor}
		must(s.CompleteManagement(ctx, project.ID, task.ID, rev.Revision, job.Lease, review))
		draft, err := s.Task(ctx, task.ID)
		must(err)
		if draft.Status != "PLANNING" || draft.Spec.Policy.Management == floor || draft.Plan.Review.ValidationMinimum != floor {
			t.Fatal("review rewrote the active policy before activation")
		}
		active, err := s.ActivatePlan(ctx, task.ID, rev.Revision)
		must(err)
		want := []string{"V3", "V4", "V4"}[i]
		if active.Spec.Policy.Management != want {
			t.Fatal("incorrect goal minimum", active.Spec.Policy)
		}
		for _, row := range active.Children {
			child, err := s.Task(ctx, row.ID)
			must(err)
			expected := want
			if child.Status == "RUNNING" {
				expected = "V3"
			}
			if child.Spec.Policy.Management != expected {
				t.Fatalf("revision %d child %d (%s) expected %s", rev.Revision, row.ID, child.Status, expected)
			}
		}
		if i == 0 {
			job, err := s.Claim(ctx, "dev", t.TempDir(), true, true, false, true, true, true, true)
			must(err)
			if job == nil || job.Spec.Policy.Management != "V3" {
				t.Fatal("missing initial attempt")
			}
		}
	}
}
