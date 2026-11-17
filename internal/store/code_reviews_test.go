package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"forgeos/internal/review"
)

func TestCodeReviewGate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	conflict := func(err error) {
		t.Helper()
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	base, commit := strings.Repeat("a", 40), strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{RepositoryID: 6, ID: 7, Number: 1, Title: "Bound the queue", Body: "Count must be below the limit."}, BaseCommit: base, Validation: "docs"})
	must(err)
	job, err := s.Claim(ctx, "dev", t.TempDir(), true, true)
	must(err)
	if job == nil {
		t.Fatal("missing development job")
	}
	ex := job.ExecutionID
	must(s.Complete(ctx, ex, job.LeaseToken, Result{BaseCommit: base, Commit: commit, SessionID: "dev-session", Validation: &ValidationResult{Profile: "docs", Commit: commit}}))
	approve := func() error {
		_, err := s.Review(ctx, task.ID, ex, commit, "approve", "checked", false, "")
		return err
	}
	must(s.SetCodeReviewPolicy(ctx, project.ID, true))
	checkState := func(want string) {
		t.Helper()
		stored, err := s.Task(ctx, task.ID)
		must(err)
		if !stored.RequiresCodeReview || stored.Execution.CodeReviewState != want {
			t.Fatalf("task detail review state %q, want %q", stored.Execution.CodeReviewState, want)
		}
	}
	checkState("")
	conflict(approve())
	conflict(s.RequestCodeReview(ctx, task.ID, ex, base))
	claim := func(projectID int64) *ReviewJob {
		t.Helper()
		j, err := s.ClaimCodeReview(ctx, projectID, false)
		must(err)
		return j
	}
	if claim(project.ID+1) != nil {
		t.Fatal("wrong project claimed job")
	}
	j := claim(project.ID)
	if j == nil || j.ExecutionID != ex || j.Commit != commit || !strings.Contains(j.Requirement, "below the limit") || claim(project.ID) != nil {
		t.Fatal("missing or duplicated review lease")
	}
	must(s.FailCodeReview(ctx, project.ID, ex, j.Lease))
	checkState("failed")
	if claim(project.ID) != nil {
		t.Fatal("failed review retried without request")
	}
	conflict(approve())
	must(s.RequestCodeReview(ctx, task.ID, ex, commit))
	checkState("pending")
	retry := claim(project.ID)
	if retry == nil || retry.Lease == j.Lease {
		t.Fatal("missing new lease")
	}
	conflict(s.FailCodeReview(ctx, project.ID, ex, j.Lease))
	report := review.Report{Base: base, Commit: commit, Session: "dev-session", Summary: "Review complete.", Findings: []review.Finding{{ID: "R1", EvidenceVerified: false}}}
	complete := func(projectID int64, lease string) error {
		return s.CompleteCodeReview(ctx, projectID, ex, lease, report)
	}
	conflict(complete(project.ID, retry.Lease))
	report.Session = "review-session"
	conflict(complete(project.ID+1, retry.Lease))
	conflict(complete(project.ID, j.Lease))
	must(complete(project.ID, retry.Lease))
	must(complete(project.ID, retry.Lease))
	report.Summary = "Modified"
	checkState("needs_check")
	conflict(complete(project.ID, retry.Lease))
	conflict(approve())
	conflict(s.CheckReviewFindings(ctx, task.ID, ex, base, "wrong candidate"))
	conflict(s.CheckReviewFindings(ctx, task.ID, ex, commit, " "))
	must(s.CheckReviewFindings(ctx, task.ID, ex, commit, "Read the referenced line and confirmed the proposed finding does not apply."))
	must(approve())
	checkState("clear")
	receipt, err := s.ReviewReceipt(ctx, task.ID, ex)
	must(err)
	if strings.Contains(string(receipt), retry.Lease) || !strings.Contains(string(receipt), "review-session") {
		t.Fatal("unexpected receipt")
	}
}

func TestReviewerUsesTheCandidatesOriginalRules(t *testing.T) {
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
	base := strings.Repeat("a", 40)
	sha := strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, ID: 1, Number: 1, Title: "Heading"}, BaseCommit: base, Validation: "docs"})
	must(err)
	published, err := s.PublishGlobalRules(ctx, 0, "First title")
	must(err)
	dev := func(commit string) int64 {
		t.Helper()
		j, err := s.ClaimForProjects(ctx, "dev", t.TempDir(), []int64{p.ID}, WorkerCapabilities{SupportsCodex: true, SupportsRework: true, SupportsGlobalRules: true})
		must(err)
		if j == nil {
			t.Fatal("missing candidate")
		}
		must(s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{BaseCommit: base, Commit: commit, SessionID: "development-" + commit, Validation: &ValidationResult{Profile: "docs", Commit: commit}}))
		return j.ExecutionID
	}
	first := dev(sha)
	_, err = s.Review(ctx, task.ID, first, sha, "reject", "Use the Owner heading", false, "")
	must(err)
	sha = strings.Repeat("c", 40)
	second := dev(sha)
	_, err = s.PublishGlobalRules(ctx, 1, "Later title")
	must(err)
	must(s.RequestCodeReview(ctx, task.ID, second, sha))
	j, err := s.ClaimCodeReview(ctx, p.ID, false)
	must(err)
	if j != nil {
		t.Fatal("legacy reviewer received rules it cannot use")
	}
	j, err = s.ClaimCodeReview(ctx, p.ID, true)
	must(err)
	if j == nil {
		t.Fatal("compatible reviewer did not receive candidate")
	}
	var evidence struct {
		GlobalRules json.RawMessage `json:"global_rules"`
		Feedback    Review          `json:"acceptance_feedback"`
		Decisions   json.RawMessage `json:"decisions"`
	}
	must(json.Unmarshal(j.CandidateContext, &evidence))
	if string(evidence.GlobalRules) != string(published) || evidence.Feedback.Note != "Use the Owner heading" || evidence.Feedback.ExecutionID != first || evidence.Decisions != nil {
		t.Fatalf("unexpected review context: %s", j.CandidateContext)
	}
	receipt, err := s.ReviewReceipt(ctx, task.ID, second)
	must(err)
	var archived ReviewJob
	must(json.Unmarshal(receipt, &archived))
	if string(archived.CandidateContext) != string(j.CandidateContext) {
		t.Fatal("receipt differs from review dispatch")
	}
}
