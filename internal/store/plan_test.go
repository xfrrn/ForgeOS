package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPlannerLeaseCannotDeliverCode(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "planner repo")
	if err != nil {
		t.Fatal(err)
	}
	base := strings.Repeat("a", 40)
	spec := IssueSpec{ImportOrigin: "owner", Mode: "plan", ChildHarness: "claude", ProjectConfigBlob: base, Issue: Issue{ID: 17, Number: 2, RepositoryID: 9}, BaseCommit: base, Validation: "docs"}
	task, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	spec.ImportOrigin = "github"
	if _, err := s.ImportIssue(ctx, p.ID, spec); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if legacy, err := s.Claim(ctx, "old", root, true, true); err != nil || legacy != nil {
		t.Fatalf("legacy worker: %v %v", legacy, err)
	}
	job, err := s.Claim(ctx, "planner", root, true, true, true)
	if err != nil || job == nil || job.IssueImportOrigin != "owner" {
		t.Fatal(job, err)
	}
	candidate := strings.Repeat("b", 40)
	result := Result{BaseCommit: base, Commit: candidate, SessionID: "planning", Validation: &ValidationResult{Commit: candidate, Profile: "docs"}}
	if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); !errors.Is(err, ErrConflict) {
		t.Fatal("planner delivered code", err)
	}
	plan := Plan{Objective: "Handbook", Milestones: []Milestone{{Name: "Docs", Items: []PlanItem{{Key: "usage", Objective: "usage", Scope: "README", Acceptance: "example present", Deliverables: "README"}}}}}
	result = Result{BaseCommit: base, SessionID: "planning", Plan: &plan}
	for range 2 {
		if err := s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Task(ctx, task.ID)
	if err != nil || got.Status != "PLANNING" || got.Plan == nil || got.Plan.Revision != 1 || len(got.Children) != 0 || len(got.Events) != 3 || got.Events[2].PlanRevision != 1 {
		t.Fatalf("unexpected draft: %+v %v", got, err)
	}
	if _, err := s.ActivatePlan(ctx, task.ID, 1); err != nil {
		t.Fatal(err)
	}
	if child, err := s.Claim(ctx, "codex", root, true); err != nil || child != nil {
		t.Fatalf("child lost project harness: %v %v", child, err)
	}
	child, err := s.Claim(ctx, "writer", root, false, true, false, false, true, true, true, true)
	if err != nil || child == nil || child.Spec.Mode != "" || child.Spec.ChildHarness != "" || child.Spec.Harness != "claude" || child.Spec.ProjectConfigBlob != base {
		t.Fatalf("child inherited planning mode: %v %v", child, err)
	}
}

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
	other, err := s.CreateProject(ctx, "other repo")
	if err != nil {
		t.Fatal(err)
	}
	decision := func(project int64, key, state string) (int64, string) {
		t.Helper()
		raw, err := s.ProposeDecision(ctx, project, DecisionProposal{RequestKey: key, Statement: key, Source: "Owner minutes",
			Validity: "Handbook", EvidenceRefs: []string{"minutes/1"}})
		var d struct{ ID int64 }
		if err != nil || json.Unmarshal(raw, &d) != nil {
			t.Fatal(string(raw), err)
		}
		if state != "PROPOSED" {
			if _, err = s.Decide(ctx, project, d.ID, state); err != nil {
				t.Fatal(err)
			}
		}
		return d.ID, fmt.Sprintf("/projects/%d/decisions/%d", project, d.ID)
	}
	confirmed, ref := decision(p.ID, "retained for this child", "CONFIRMED")
	_, pending := decision(p.ID, "unconfirmed private draft", "PROPOSED")
	_, foreign := decision(other.ID, "foreign private decision", "CONFIRMED")
	second.ContextRefs = []string{"README.md", ref, pending, foreign, ref}
	plan := Plan{Objective: "Document usage", Milestones: []Milestone{{"Handbook", []PlanItem{second, first}}, {Name: "Later"}}}
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
	if _, err := s.ActivatePlan(ctx, task.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("activated superseded plan: %v", err)
	}
	for range 2 {
		active, err := s.ActivatePlan(ctx, task.ID, 2)
		if err != nil || active.Status != "ACTIVE" || len(active.Children) != 2 || len(active.Events) != 6 {
			t.Fatalf("activation/replay: %+v %v", active, err)
		}
	}
	child, err := s.Claim(ctx, "worker", root, true)
	if err != nil || child == nil || child.TaskID == task.ID || child.Spec.Issue.Title != first.Objective {
		t.Fatalf("incorrect ready item: %+v %v", child, err)
	}
	detail, err := s.Task(ctx, child.TaskID)
	if err != nil || detail.GoalID != task.ID || detail.PlanRevision != 2 || detail.PlanKey != "a" {
		t.Fatalf("missing lineage: %+v %v", detail, err)
	}
	if next, err := s.NextDependent(ctx, p.ID, 0); next != nil || err != nil {
		t.Fatalf("released before merge: %+v %v", next, err)
	}
	candidate, merged := strings.Repeat("b", 40), strings.Repeat("c", 40)
	if err := s.Complete(ctx, child.ExecutionID, child.LeaseToken, Result{Commit: candidate, BaseCommit: child.Spec.BaseCommit,
		SessionID: "first-child", Validation: &ValidationResult{Commit: candidate, Profile: "docs"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(ctx, child.TaskID, child.ExecutionID, candidate, "approve", "reviewed", true, "main"); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimDelivery(ctx, p.ID)
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	d.PRNumber, d.PRURL, d.MergeSHA, d.State = 5, "https://github.com/example/repo/pull/5", merged, "DONE"
	if err := s.SaveDelivery(ctx, *d); err != nil {
		t.Fatal(err)
	}
	next, err := s.NextDependent(ctx, p.ID, 0)
	if err != nil || next == nil || len(next.Merges) != 1 || next.Merges[0] != merged || next.Target != "main" {
		t.Fatalf("release evidence: %+v %v", next, err)
	}
	// Revise the undelivered child after its prerequisite merged but before a remote base check returns.
	saved, err := s.PlanRevision(ctx, task.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	rolling := saved.Plan
	rolling.Milestones[0].Items[0].Acceptance = "The updated example links to the first"
	transient := first
	transient.Key, transient.Dependencies = "provisional", []string{"b"}
	rolling.Milestones = append(rolling.Milestones, Milestone{Name: "Future", Items: []PlanItem{transient}})
	if _, err = s.SavePlan(ctx, task.ID, 2, rolling); err != nil {
		t.Fatal(err)
	}
	if c, err := s.ClaimGoalClosure(ctx, p.ID); err != nil || c != nil {
		t.Fatal(c, err)
	}
	active, err := s.ActivatePlan(ctx, task.ID, 3)
	if err != nil || active.ActiveRevision != 3 || len(active.Children) != 3 {
		t.Fatal(active, err)
	}
	if err = s.ReleaseDependent(ctx, next.TaskID, next.Revision, merged); !errors.Is(err, ErrConflict) {
		t.Fatal("stale dependency check released revised work", err)
	}
	rolling.Milestones = rolling.Milestones[:1]
	if _, err = s.SavePlan(ctx, task.ID, 3, rolling); err != nil {
		t.Fatal(err)
	}
	active, err = s.ActivatePlan(ctx, task.ID, 4)
	if err != nil || active.Children[2].Status != "SUPERSEDED" {
		t.Fatal(active, err)
	}
	preserved, err := s.Task(ctx, child.TaskID)
	if err != nil || preserved.Status != "DONE" || preserved.PlanRevision != 2 || preserved.Execution.ID != child.ExecutionID || preserved.Delivery.MergeSHA != merged {
		t.Fatal(preserved, err)
	}
	rolling.Milestones[0].Items[1].Scope = "rewrite delivered work"
	if _, err = s.SavePlan(ctx, task.ID, 4, rolling); !errors.Is(err, ErrConflict) {
		t.Fatal("changed merged item", err)
	}
	rolling.Milestones[0].Items[1].Scope = first.Scope
	next, err = s.NextDependent(ctx, p.ID, 0)
	if err != nil || next == nil || next.Revision != 3 {
		t.Fatal(next, err)
	}
	if err := s.ReleaseDependent(ctx, next.TaskID, next.Revision, merged); err != nil {
		t.Fatal(err)
	}
	secondJob, err := s.Claim(ctx, "worker", root, true)
	if err != nil || secondJob == nil || secondJob.TaskID != next.TaskID || secondJob.Spec.BaseCommit != merged {
		t.Fatalf("did not freeze delivered base: %+v %v", secondJob, err)
	}
	var evidence struct {
		PlanRevision int `json:"plan_revision"`
		Requirement  RequirementRevision
		Dependencies []Delivery
		Decisions    []struct {
			ID             int64
			Ref, Statement string
		}
	}
	if err := json.Unmarshal(secondJob.GoalContext, &evidence); err != nil || evidence.PlanRevision != 3 ||
		evidence.Requirement.TaskID != task.ID || evidence.Requirement.Revision != 1 ||
		len(evidence.Dependencies) != 1 || evidence.Dependencies[0].MergeSHA != merged ||
		len(evidence.Decisions) != 1 || evidence.Decisions[0].ID != confirmed || evidence.Decisions[0].Ref != ref || evidence.Decisions[0].Statement != "retained for this child" {
		t.Fatalf("missing child claim evidence: %s %v", secondJob.GoalContext, err)
	}
	if strings.Contains(string(child.GoalContext), "retained for this child") || strings.Contains(string(secondJob.GoalContext), "foreign private decision") {
		t.Fatal("unassigned decision in claim")
	}
	if _, err := s.Decide(ctx, p.ID, confirmed, "EXPIRED"); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.Task(ctx, secondJob.TaskID)
	if err != nil || claimed.Execution.DecisionContext == nil || !strings.Contains(string(*claimed.Execution.DecisionContext), "retained for this child") {
		t.Fatal("lost execution decision snapshot", err)
	}
	if c, err := s.ClaimGoalClosure(ctx, p.ID); c != nil || err != nil {
		t.Fatalf("premature parent closure: %v %v", c, err)
	}
	candidate = strings.Repeat("d", 40)
	if err := s.Complete(ctx, secondJob.ExecutionID, secondJob.LeaseToken, Result{Commit: candidate, BaseCommit: merged,
		SessionID: "second-child", Validation: &ValidationResult{Commit: candidate, Profile: "docs"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Review(ctx, secondJob.TaskID, secondJob.ExecutionID, candidate, "approve", "reviewed", true, "main"); err != nil {
		t.Fatal(err)
	}
	d, err = s.ClaimDelivery(ctx, p.ID)
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	d.PRNumber, d.PRURL, d.MergeSHA, d.State = 6, "https://github.com/example/repo/pull/6", strings.Repeat("e", 40), "DONE"
	if err := s.SaveDelivery(ctx, *d); err != nil {
		t.Fatal(err)
	}
	c, err := s.ClaimGoalClosure(ctx, p.ID)
	if err != nil || c == nil || c.ID != task.ID {
		t.Fatal(c, err)
	}
	if duplicate, err := s.ClaimGoalClosure(ctx, p.ID); duplicate != nil || err != nil {
		t.Fatalf("duplicate closure claim: %v %v", duplicate, err)
	}
	if _, err = s.SavePlan(ctx, task.ID, 4, rolling); !errors.Is(err, ErrConflict) {
		t.Fatal("reopened a goal with closure in flight", err)
	}
	if hits, err := s.SearchHistory(ctx, p.ID, task.ID, "goal", ""); err != nil || len(hits) != 0 {
		t.Fatalf("unfinished Goal entered completed history: %v %v", hits, err)
	}
	if err := s.CompleteGoal(ctx, *c); err != nil {
		t.Fatal(err)
	}
	parent, err := s.Task(ctx, task.ID)
	if err != nil || parent.Status != "DONE" || parent.Events[len(parent.Events)-1].Type != "goal_delivered" {
		t.Fatalf("parent completion: %v %v", parent, err)
	}
	if hits, err := s.SearchHistory(ctx, p.ID, task.ID, "goal", rolling.Objective); err != nil || len(hits) != 1 ||
		hits[0].SourceID != task.ID || hits[0].Version != int64(parent.ActiveRevision) ||
		!hits[0].At.Equal(parent.Events[len(parent.Events)-1].At) {
		t.Fatalf("missing completed Goal revision: %v %v", hits, err)
	}
	event := parent.Events[len(parent.Events)-1].ID
	receipt, err := s.EventCuration(ctx, p.ID, task.ID, event)
	var completed struct {
		Goal struct {
			Revision int `json:"plan_revision"`
			Children []struct{ Key string }
		}
	}
	if err != nil || json.Unmarshal(receipt, &completed) != nil || completed.Goal.Revision != 4 || len(completed.Goal.Children) != 2 {
		t.Fatal(string(receipt), err)
	}
	audit, err := s.EventAudit(ctx, p.ID, task.ID, event)
	var recorded struct {
		Payload struct {
			Goal json.RawMessage `json:"completed_goal"`
		}
	}
	var expected struct{ Goal json.RawMessage }
	if err != nil || json.Unmarshal(audit, &recorded) != nil || json.Unmarshal(receipt, &expected) != nil || string(expected.Goal) != string(recorded.Payload.Goal) {
		t.Fatal("audit must identify the same completed children", string(audit), err)
	}
	var source string
	if err := s.db.QueryRowContext(ctx, "SELECT content FROM curation_sources WHERE kind='goal_delivered' AND revision=$1", event).Scan(&source); err != nil ||
		!strings.Contains(source, "Candidate base matched the confirmed merge of prerequisite a.") || strings.Contains(source, "provisional") {
		t.Fatal(source, err)
	}
	if err := s.CompleteGoal(ctx, *c); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate completion", err)
	}
	if again, err := s.EventCuration(ctx, p.ID, task.ID, event); err != nil || string(again) != string(receipt) {
		t.Fatal("changed completion snapshot", err)
	}
	raw, err := s.TaskContext(ctx, p.ID, secondJob.TaskID)
	var contextView struct {
		Plan         PlanRevision
		Requirement  RequirementRevision
		Decisions    []json.RawMessage
		Dependencies []struct {
			Key      string
			MergeSHA string `json:"merge_sha"`
		}
	}
	if err != nil || json.Unmarshal(raw, &contextView) != nil || contextView.Plan.Revision != 3 ||
		contextView.Requirement.TaskID != task.ID || contextView.Requirement.Revision != 1 || len(contextView.Decisions) != 0 ||
		len(contextView.Dependencies) != 1 || contextView.Dependencies[0].Key != "a" || contextView.Dependencies[0].MergeSHA != merged {
		t.Fatalf("child context lost its frozen sources or merged prerequisite: %s %v", raw, err)
	}
	if _, err := s.TaskContext(ctx, p.ID+1, secondJob.TaskID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("returned another project's task context: %v", err)
	}
}
