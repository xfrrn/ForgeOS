package store

import (
	"context"
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
	conflict(approve())
	conflict(s.RequestCodeReview(ctx, task.ID, ex, base))
	claim := func(projectID int64) *ReviewJob {
		t.Helper()
		j, err := s.ClaimCodeReview(ctx, projectID)
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
	if claim(project.ID) != nil {
		t.Fatal("failed review retried without request")
	}
	conflict(approve())
	must(s.RequestCodeReview(ctx, task.ID, ex, commit))
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
	conflict(complete(project.ID, retry.Lease))
	conflict(approve())
	conflict(s.CheckReviewFindings(ctx, task.ID, ex, base, "wrong candidate"))
	conflict(s.CheckReviewFindings(ctx, task.ID, ex, commit, " "))
	must(s.CheckReviewFindings(ctx, task.ID, ex, commit, "Read the referenced line and confirmed the proposed finding does not apply."))
	must(approve())
	receipt, err := s.ReviewReceipt(ctx, task.ID, ex)
	must(err)
	if strings.Contains(string(receipt), retry.Lease) || !strings.Contains(string(receipt), "review-session") {
		t.Fatal("unexpected receipt")
	}
}
