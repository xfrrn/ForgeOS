package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestIssueWorkTypeRevisionAndClarification(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, "classified")
	if err != nil {
		t.Fatal(err)
	}
	issue := Issue{ID: 10, Number: 1, RepositoryID: 12, Title: "Release", WorkType: "development", OwnerClassified: true, UpdatedAt: "2026-01-01T00:00:00Z"}
	spec := IssueSpec{ImportOrigin: "owner", Issue: issue, Harness: "claude", BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	task, err := s.ImportIssue(ctx, project.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	other := spec
	other.ImportOrigin = "github"
	if _, err = s.ImportIssue(ctx, project.ID, other); err != nil {
		t.Fatal(err)
	}
	importEvent := task.Events[0].ID
	original, err := s.EventAudit(ctx, project.ID, task.ID, importEvent)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Source  string         `json:"source"`
		Payload map[string]any `json:"payload"`
	}
	if err = json.Unmarshal(original, &receipt); err != nil || receipt.Source != "owner" || receipt.Payload["work_type"] != "development" ||
		receipt.Payload["requirement_revision"] != float64(1) || receipt.Payload["issue_number"] != float64(issue.Number) {
		t.Fatal(string(original), err)
	}
	other.Issue.WorkType, other.Mode, other.Harness = "testing", "plan", "codex"
	if _, err := s.ImportIssue(ctx, project.ID, other); !errors.Is(err, ErrConflict) {
		t.Fatal("conflicting concurrent Owner decision accepted", err)
	}
	issue.WorkType, issue.OwnerClassified = "clarification", false
	if _, err := s.ObserveIssue(ctx, project.ID, issue); err != nil {
		t.Fatal(err)
	}
	if task, err = s.Task(ctx, task.ID); err != nil || task.LatestRequirement != 1 || task.Spec.Issue.WorkType != "development" {
		t.Fatal("poll replaced Owner clarification", err)
	}
	issue.Title = "New scope"
	if _, err = s.ObserveIssue(ctx, project.ID, issue); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdoptRequirement(ctx, task.ID, 2); !errors.Is(err, ErrConflict) {
		t.Fatal("unresolved revision dispatched", err)
	}
	issue.WorkType = "operations"
	if _, err = s.ObserveIssue(ctx, project.ID, issue); err != nil {
		t.Fatal(err)
	}
	task, err = s.AdoptRequirement(ctx, task.ID, 3)
	if err != nil || task.Spec.Mode != "plan" || task.Spec.ChildHarness != "claude" || !task.Spec.ValidHarness() || task.Spec.Issue.WorkType != "operations" {
		t.Fatal("new scope did not enter read-only planning", err)
	}
	initial, err := s.RequirementRevision(ctx, task.ID, 1)
	if err != nil || !initial.Issue.OwnerClassified || initial.Issue.WorkType != "development" {
		t.Fatal("lost original classification", err)
	}
	issue.WorkType, issue.WorkTypeLabels = "clarification", "operations,testing"
	if _, err = s.ObserveIssue(ctx, project.ID, issue); err != nil {
		t.Fatal(err)
	}
	issue.WorkTypeLabels = "code-review,operations"
	if _, err = s.ObserveIssue(ctx, project.ID, issue); err != nil {
		t.Fatal(err)
	}
	observed, err := s.RequirementRevision(ctx, task.ID, 5)
	if err != nil || observed.Issue.WorkTypeLabels != issue.WorkTypeLabels {
		t.Fatal("different conflicting labels lost their revision", err)
	}
	after, err := s.EventAudit(ctx, project.ID, task.ID, importEvent)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("later revisions changed the import receipt", string(after), err)
	}
}

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
	raw, err := s.OpenPreview(ctx, task.ID, job.ExecutionID, candidate, "public", 8091, 30)
	var preview struct{ ID string }
	if err != nil || json.Unmarshal(raw, &preview) != nil {
		t.Fatal(err, string(raw))
	}
	fid, err := s.SubmitPreviewFeedback(ctx, task.ID, preview.ID, PreviewFeedback{Request: "ee08bcfe-0011-4d87-8739-c0eb1613f539",
		Execution: job.ExecutionID, Commit: candidate, Path: "/", Note: "Change the rule."})
	if err != nil {
		t.Fatal(err)
	}
	change := FeedbackRequirement{FeedbackID: fid, Execution: job.ExecutionID, Commit: candidate, Previous: 3, Body: "Owner changed the capacity rule."}
	if _, err = s.ReviseFromFeedback(ctx, task.ID, preview.ID, change); !errors.Is(err, ErrConflict) {
		t.Fatal("unclassified feedback accepted", err)
	}
	review, err := s.ClaimFeedback(ctx, p.ID, true, false)
	if err != nil || review == nil {
		t.Fatal(review, err)
	}
	if err = s.ClassifyFeedback(ctx, p.ID, fid, review.Lease, FeedbackClassification{Category: "requirement_change", Session: "management", Rationale: "Changes scope", Evidence: "Change the rule."}); err != nil {
		t.Fatal(err)
	}
	stale := change
	stale.Previous--
	if _, err = s.ReviseFromFeedback(ctx, task.ID, preview.ID, stale); !errors.Is(err, ErrConflict) {
		t.Fatal("stale draft", err)
	}
	for range 2 {
		revision, err := s.ReviseFromFeedback(ctx, task.ID, preview.ID, change)
		if err != nil || revision.Revision != 4 || revision.FeedbackID != fid || revision.FeedbackTask != task.ID || revision.FeedbackPreview != preview.ID || revision.Issue.Body != change.Body {
			t.Fatal(revision, err)
		}
	}
	if _, err = s.ObserveIssue(ctx, p.ID, issue); err != nil {
		t.Fatal(err)
	}
	current, err := s.Task(ctx, task.ID)
	if err != nil || current.LatestRequirement != 4 || current.Status != "RUNNING" || current.Execution.RequirementRevision != 3 {
		t.Fatal("poll or feedback changed running work", current, err)
	}
	issue.Body = "new GitHub scope"
	if _, err = s.ObserveIssue(ctx, p.ID, issue); err != nil {
		t.Fatal(err)
	}
	remote, err := s.RequirementRevision(ctx, task.ID, 5)
	if err != nil || remote.FeedbackID != 0 || remote.Issue.Body != issue.Body {
		t.Fatal("missed real GitHub change", remote, err)
	}
	current, err = s.Task(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var revisions []int
	for _, event := range current.Events {
		if event.Type != "requirement_observed" && event.Type != "requirement_adopted" && event.Type != "requirement_revised_from_feedback" {
			continue
		}
		raw, err := s.EventAudit(ctx, p.ID, task.ID, event.ID)
		if err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			Source  string         `json:"source"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &receipt); err != nil {
			t.Fatal(err)
		}
		version, ok := receipt.Payload["requirement_revision"].(float64)
		if !ok {
			t.Fatal(event.Type, string(raw))
		}
		revisions = append(revisions, int(version))
		wantSource := "owner"
		if event.Type == "requirement_observed" {
			wantSource = "github"
		}
		if receipt.Source != wantSource {
			t.Fatal(event.Type, string(raw))
		}
		if event.Type == "requirement_revised_from_feedback" && (receipt.Payload["feedback_id"] != float64(fid) ||
			receipt.Payload["feedback_execution_id"] != float64(job.ExecutionID) || receipt.Payload["commit"] != candidate) {
			t.Fatal("lost feedback candidate", string(raw))
		}
	}
	if !slices.Equal(revisions, []int{2, 3, 3, 4, 5}) {
		t.Fatalf("unexpected requirement events: %v", revisions)
	}
	change.Body = "different replay"
	if _, err = s.ReviseFromFeedback(ctx, task.ID, preview.ID, change); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay", err)
	}
}
