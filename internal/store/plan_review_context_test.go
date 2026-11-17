package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestPlanReviewFreezesOnlyReferencedProjectRules(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, "review-context")
	must(err)
	other, err := s.CreateProject(ctx, "other-context")
	must(err)
	decision := func(project int64, key, state string) string {
		t.Helper()
		doc, e := s.ProposeDecision(ctx, project, DecisionProposal{RequestKey: key, Statement: key, Source: "Owner", Validity: "reader guide", EvidenceRefs: []string{"docs/guide.md"}})
		must(e)
		var row struct{ ID int64 }
		must(json.Unmarshal(doc, &row))
		if state != "PROPOSED" {
			_, e = s.Decide(ctx, project, row.ID, "CONFIRMED")
			must(e)
		}
		if state == "EXPIRED" {
			_, e = s.Decide(ctx, project, row.ID, "EXPIRED")
			must(e)
		}
		return fmt.Sprintf("/projects/%d/decisions/%d", project, row.ID)
	}
	valid := decision(p.ID, "Project heading", "CONFIRMED")
	refs := []string{valid, valid, decision(other.ID, "Private other project", "CONFIRMED"), decision(p.ID, "Expired heading", "EXPIRED"), decision(p.ID, "Proposed heading", "PROPOSED"), "docs/guide.md"}
	decision(p.ID, "Unreferenced heading", "CONFIRMED")
	rules, err := s.PublishGlobalRules(ctx, 0, "Default heading")
	must(err)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 1, Number: 1, RepositoryID: 9, Body: "Owner heading"}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	must(err)
	item := PlanItem{Key: "first", Objective: "Guide", Scope: "docs", Deliverables: "guide", Acceptance: "Owner heading", ContextRefs: refs}
	second := item
	second.Key, second.ContextRefs = "second", []string{valid}
	plan := Plan{Objective: "Reader guide", Milestones: []Milestone{{Name: "Guides", Items: []PlanItem{item, second}}}}
	_, err = s.SavePlan(ctx, task.ID, 0, plan)
	must(err)
	job, err := s.ClaimManagement(ctx, p.ID, true, true)
	must(err)
	if job == nil {
		t.Fatal("missing review")
	}
	var evidence struct {
		Rules     json.RawMessage `json:"global_rules"`
		Decisions []struct {
			ID             int64
			Ref, Statement string
			Keys           []string `json:"plan_keys"`
		} `json:"decisions"`
	}
	must(json.Unmarshal(job.ReviewContext, &evidence))
	if string(evidence.Rules) != string(rules) || len(evidence.Decisions) != 1 || evidence.Decisions[0].Ref != valid ||
		evidence.Decisions[0].Statement != "Project heading" || !reflect.DeepEqual(evidence.Decisions[0].Keys, []string{"first", "second"}) {
		t.Fatal("incorrect scope or rule revision", string(job.ReviewContext))
	}
	_, err = s.PublishGlobalRules(ctx, 1, "Future heading")
	must(err)
	_, err = s.Decide(ctx, p.ID, evidence.Decisions[0].ID, "EXPIRED")
	must(err)
	review := ManagementReview{Verdict: "approve", Level: "D2", Rationale: "Owner heading wins", Evidence: valid, SessionID: "review-session"}
	must(s.CompleteManagement(ctx, p.ID, task.ID, 1, job.Lease, review))
	for range 2 {
		stored, e := s.PlanRevision(ctx, task.ID, 1)
		must(e)
		if stored.ReviewContext == nil || string(*stored.ReviewContext) != string(job.ReviewContext) {
			t.Fatal("changed completed review evidence", stored)
		}
		must(s.CompleteManagement(ctx, p.ID, task.ID, 1, job.Lease, review))
	}
	_, err = s.SavePlan(ctx, task.ID, 1, plan)
	must(err)
	claim := func(supported bool) *ManagementJob {
		t.Helper()
		j, e := s.ClaimManagement(ctx, p.ID, true, supported)
		must(e)
		if j == nil {
			t.Fatal("missing reclaimed review")
		}
		return j
	}
	expire := func() {
		_, e := s.db.ExecContext(ctx, "UPDATE plan_revisions SET review_until=now() WHERE task_id=$1 AND revision=2", task.ID)
		must(e)
	}
	legacy := claim(false)
	if legacy.ReviewContext != nil {
		t.Fatal("legacy worker received new context")
	}
	expire()
	next := claim(true)
	must(json.Unmarshal(next.ReviewContext, &evidence))
	if len(evidence.Decisions) != 0 || !strings.Contains(string(evidence.Rules), "Future heading") {
		t.Fatal("reused old context", string(next.ReviewContext))
	}
	expire()
	legacy = claim(false)
	if legacy.ReviewContext != nil {
		t.Fatal("claimed context for a worker that cannot read it")
	}
	must(s.CompleteManagement(ctx, p.ID, task.ID, 2, legacy.Lease, review))
	stored, err := s.PlanRevision(ctx, task.ID, 2)
	must(err)
	if stored.ReviewContext != nil {
		t.Fatal("legacy review retained unused context")
	}
	old, err := s.PlanRevision(ctx, task.ID, 1)
	must(err)
	if old.ReviewContext == nil || string(*old.ReviewContext) != string(job.ReviewContext) {
		t.Fatal("new claim rewrote historical review")
	}
}
