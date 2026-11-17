package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFeedbackContextPinsRequirementRulesAndAssignedItem(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	issue := Issue{ID: 3, Number: 1, RepositoryID: 2, Title: "Queue guide", Body: "Use Owner footer", UpdatedAt: "2026-01-01T09:00:00Z"}
	base, sha := strings.Repeat("a", 40), strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: issue, BaseCommit: base, Validation: "docs"})
	must(err)
	plan := Plan{Objective: "Guide", Milestones: []Milestone{{Name: "Docs", Items: []PlanItem{{Key: "guide", Objective: "Write guide", Scope: "guide.md only", Acceptance: "Owner footer", Deliverables: "guide.md"}}}}}
	_, err = s.SavePlan(ctx, task.ID, 0, plan)
	must(err)
	goal, err := s.ActivatePlan(ctx, task.ID, 1)
	must(err)
	child := goal.Children[0].ID
	rules, err := s.PublishGlobalRules(ctx, 0, "Fallback footer")
	must(err)
	job, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), nil, WorkerCapabilities{SupportsCodex: true, SupportsGlobalRules: true})
	must(err)
	if job == nil || job.TaskID != child {
		t.Fatal("missing child")
	}
	must(s.Complete(ctx, job.ExecutionID, job.LeaseToken, Result{BaseCommit: base, Commit: sha, SessionID: "implementation", Validation: &ValidationResult{Commit: sha, Profile: "docs"}}))
	pv, err := s.OpenPreview(ctx, child, job.ExecutionID, sha, "docs", 8091, 60)
	must(err)
	var preview struct{ ID string }
	must(json.Unmarshal(pv, &preview))
	f := PreviewFeedback{Request: "09b8c87f-4ee6-4a9f-9b44-aa7bc28332fc", Execution: job.ExecutionID, Commit: sha, Path: "/", Note: "Change Owner footer to Another footer"}
	id, err := s.SubmitPreviewFeedback(ctx, child, preview.ID, f)
	must(err)
	issue.Body, issue.UpdatedAt = "Later requirement", "2026-01-02T09:00:00Z"
	_, err = s.ObserveIssue(ctx, p.ID, issue)
	must(err)
	_, err = s.PublishGlobalRules(ctx, 1, "Later fallback")
	must(err)
	claim := func(supports bool) *FeedbackJob {
		t.Helper()
		j, err := s.ClaimFeedback(ctx, p.ID, true, supports)
		must(err)
		if j == nil || j.ID != id {
			t.Fatal("no feedback")
		}
		return j
	}
	j := claim(true)
	var source struct {
		Context struct {
			Candidate struct {
				Execution int64  `json:"execution_id"`
				Base      string `json:"base_commit"`
				Commit    string
			}
			Requirement struct {
				Revision int
				Issue    Issue
			}
			Assigned struct {
				Goal     int64 `json:"goal_id"`
				Revision int   `json:"plan_revision"`
				Item     PlanItem
			} `json:"assigned_item"`
			Rules struct {
				Global json.RawMessage `json:"global_rules"`
			}
		}
	}
	must(json.Unmarshal(j.Source, &source))
	c := source.Context
	if c.Candidate.Execution != job.ExecutionID || c.Candidate.Base != base || c.Candidate.Commit != sha || c.Requirement.Revision != 1 || c.Requirement.Issue.Body != "Use Owner footer" ||
		c.Assigned.Goal != task.ID || c.Assigned.Revision != 1 || !reflect.DeepEqual(c.Assigned.Item, plan.Milestones[0].Items[0]) || string(c.Rules.Global) != string(rules) {
		t.Fatalf("incorrect classification evidence: %s", j.Source)
	}
	if strings.Contains(string(j.Source), "Later") {
		t.Fatal("used later sources")
	}
	expire := func() {
		_, err := s.db.ExecContext(ctx, "UPDATE preview_feedback SET review_until=now() WHERE id=$1", id)
		must(err)
	}
	expire()
	old := claim(false)
	var legacy map[string]json.RawMessage
	must(json.Unmarshal(old.Source, &legacy))
	if _, ok := legacy["context"]; ok {
		t.Fatal("legacy role received unused context")
	}
	expire()
	j = claim(true)
	var raw map[string]json.RawMessage
	must(json.Unmarshal(j.Source, &raw))
	out := FeedbackClassification{Category: "requirement_change", Rationale: "Changes the Owner choice", Evidence: f.Note, Session: "management"}
	finish := func() error { return s.ClassifyFeedback(ctx, p.ID, id, j.Lease, out) }
	forged := json.RawMessage(`{}`)
	out.Context = &forged
	if !errors.Is(finish(), ErrConflict) {
		t.Fatal("accepted forged context")
	}
	out.Context = nil
	must(finish())
	must(finish())
	page, err := s.PreviewFeedbackPage(ctx, child, preview.ID)
	must(err)
	var receipt struct {
		Feedback []struct{ Classification FeedbackClassification }
	}
	must(json.Unmarshal(page, &receipt))
	if len(receipt.Feedback) != 1 || receipt.Feedback[0].Classification.Context == nil || string(*receipt.Feedback[0].Classification.Context) != string(raw["context"]) {
		t.Fatal("lost archived source")
	}
	var events int
	must(s.db.QueryRowContext(ctx, "SELECT count(*) FROM task_events WHERE task_id=$1 AND type='preview_feedback_classified'", child).Scan(&events))
	if events != 1 {
		t.Fatal("duplicate classification event")
	}
}
