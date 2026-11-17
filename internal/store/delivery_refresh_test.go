package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"forgeos/internal/review"
)

func TestRefreshKeepsDeliveryAndRequiresNewValidation(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "refresh")
	if err != nil {
		t.Fatal(err)
	}
	base, old, target, head := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40), strings.Repeat("d", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 7, Number: 1, RepositoryID: 9}, BaseCommit: base, Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	j, err := s.Claim(ctx, "model", root, true)
	if err != nil || j == nil {
		t.Fatal(j, err)
	}
	r := Result{Commit: old, BaseCommit: base, SessionID: "before", Validation: &ValidationResult{Commit: old, Profile: "docs"}}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, r); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Review(ctx, task.ID, j.ExecutionID, old, "approve", "checked", true, "main"); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimDelivery(ctx, p.ID)
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	d.State, d.PRNumber, d.PRURL = "WAITING", 4, "https://github.com/a/b/pull/4"
	if err = s.SaveDelivery(ctx, *d); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		queued, err := s.QueueRefreshedCandidate(ctx, task.ID, j.ExecutionID, old, target, head)
		if err != nil || queued.Status != "READY" || len(queued.Refreshes) != 1 || queued.Delivery != nil || queued.Spec.VerifyCommit != head {
			t.Fatal(queued, err)
		}
	}
	if _, err = s.QueueRefreshedCandidate(ctx, task.ID, j.ExecutionID, old, target, strings.Repeat("f", 40)); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay", err)
	}
	if j, err := s.Claim(ctx, "old", root, true, true, true, true); err != nil || j != nil {
		t.Fatal(j, err)
	}
	verify, err := s.Claim(ctx, "validator", root, false, false, false, false, true)
	if err != nil || verify == nil || verify.SupportsCheckpoints {
		t.Fatal(verify, err)
	}
	r.SessionID, r.BaseCommit = "", target
	if err = s.Complete(ctx, verify.ExecutionID, verify.LeaseToken, r); !errors.Is(err, ErrConflict) {
		t.Fatal("old SHA validated", err)
	}
	r.Commit, r.Validation.Commit = head, head
	if err = s.Complete(ctx, verify.ExecutionID, verify.LeaseToken, r); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.Task(ctx, task.ID)
	if err != nil || fresh.Status != "WAITING_ACCEPTANCE" || len(fresh.Reviews) != 1 || fresh.Refreshes[0].OldDelivery.Commit != old {
		t.Fatal(fresh, err)
	}
	if _, err = s.Review(ctx, task.ID, verify.ExecutionID, head, "reject", "revise this version", false, ""); err != nil {
		t.Fatal(err)
	}
	redo, err := s.Claim(ctx, "model", root, true, true)
	if err != nil || redo == nil || redo.Rework == nil || redo.Rework.Commit != head || redo.Spec.VerifyCommit != "" {
		t.Fatal(redo, err)
	}
}

func TestReviewKeepsImplementationRulesAcrossRepeatedRefresh(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, "refresh-rules")
	must(err)
	base, commit := strings.Repeat("a", 40), strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{ID: 17, Number: 1, RepositoryID: 2}, BaseCommit: base, Validation: "docs"})
	must(err)
	initial, err := s.PublishGlobalRules(ctx, 0, "Keep original")
	must(err)
	root := t.TempDir()
	job, err := s.ClaimForProjects(ctx, "dev", root, []int64{project.ID}, WorkerCapabilities{SupportsCodex: true, SupportsGlobalRules: true})
	must(err)
	if job == nil {
		t.Fatal("missing implementation")
	}
	origin, originCommit := job.ExecutionID, commit
	complete := func(session string) {
		t.Helper()
		must(s.Complete(ctx, job.ExecutionID, job.LeaseToken, Result{BaseCommit: base, Commit: commit, SessionID: session, Validation: &ValidationResult{Profile: "docs", Commit: commit}}))
	}
	complete("development")
	_, err = s.PublishGlobalRules(ctx, 1, "New instructions")
	must(err)
	for _, next := range []struct{ base, head string }{{"c", "d"}, {"e", "f"}} {
		_, err = s.Review(ctx, task.ID, job.ExecutionID, commit, "approve", "Checked candidate", true, "main")
		must(err)
		dl, err := s.ClaimDelivery(ctx, project.ID)
		must(err)
		if dl == nil {
			t.Fatal("missing delivery")
		}
		dl.State, dl.PRNumber, dl.PRURL = "WAITING", 4, "https://github.com/a/b/pull/4"
		must(s.SaveDelivery(ctx, *dl))
		previous := commit
		base, commit = strings.Repeat(next.base, 40), strings.Repeat(next.head, 40)
		_, err = s.QueueRefreshedCandidate(ctx, task.ID, job.ExecutionID, previous, base, commit)
		must(err)
		job, err = s.ClaimForProjects(ctx, "ci", root, []int64{project.ID}, WorkerCapabilities{SupportsValidation: true})
		must(err)
		if job == nil || job.GlobalRules != nil {
			t.Fatal("CI job claimed model rules")
		}
		complete("")
	}
	must(s.RequestCodeReview(ctx, task.ID, job.ExecutionID, commit))
	claimed, err := s.ClaimCodeReview(ctx, project.ID, false, false)
	must(err)
	if claimed != nil {
		t.Fatal("legacy reviewer took refreshed context")
	}
	claimed, err = s.ClaimCodeReview(ctx, project.ID, true, false)
	must(err)
	if claimed == nil {
		t.Fatal("missing refreshed review")
	}
	var got struct {
		Origin int64           `json:"implementation_execution_id"`
		Commit string          `json:"implementation_commit"`
		Rules  json.RawMessage `json:"global_rules"`
	}
	must(json.Unmarshal(claimed.CandidateContext, &got))
	if got.Origin != origin || got.Commit != originCommit || string(got.Rules) != string(initial) || claimed.Commit != commit {
		t.Fatalf("wrong implementation in %s", claimed.CandidateContext)
	}
	report := review.Report{Base: base, Commit: commit, Session: "development", Summary: "Checked refreshed change.", Findings: []review.Finding{}}
	if err = s.CompleteCodeReview(ctx, project.ID, job.ExecutionID, claimed.Lease, report); !errors.Is(err, ErrConflict) {
		t.Fatal("original development session on refreshed review", err)
	}
	report.Session = "independent-review"
	must(s.CompleteCodeReview(ctx, project.ID, job.ExecutionID, claimed.Lease, report))
	question, err := s.AskAtAcceptance(ctx, task.ID, QuestionRequest{ExecutionID: job.ExecutionID, Commit: commit, Key: "refresh-rules", Question: "Which rule applies?", Evidence: "The candidate was refreshed twice."})
	must(err)
	advice, err := s.ClaimQuestionAdvice(ctx, project.ID, false, true, true)
	must(err)
	if advice == nil {
		t.Fatal("missing refreshed advice")
	}
	must(json.Unmarshal(advice.Question.RuleContext, &got))
	if got.Origin != origin || got.Commit != originCommit || string(got.Rules) != string(initial) {
		t.Fatalf("advice lost original implementation: %s", advice.Question.RuleContext)
	}
	_, err = s.AnswerQuestion(ctx, task.ID, question.Questions[0].ID, "Keep original")
	must(err)

	_, err = s.Review(ctx, task.ID, job.ExecutionID, commit, "reject", "Clarify the handbook wording.", false, "")
	must(err)
	job, err = s.ClaimForProjects(ctx, "dev", root, []int64{project.ID}, WorkerCapabilities{SupportsCodex: true, SupportsRework: true, SupportsReworkAnswers: true, SupportsGlobalRules: true})
	must(err)
	if job == nil || job.Rework == nil || job.Rework.Commit != commit {
		t.Fatal("missing rework of refreshed candidate")
	}
	commit = strings.Repeat("9", 40)
	if err = s.RecordCICandidate(ctx, job.ExecutionID, job.LeaseToken, commit, "development"); !errors.Is(err, ErrConflict) {
		t.Errorf("rework reused the rejected implementation at CI: %v", err)
	}
	result := Result{BaseCommit: base, Commit: commit, SessionID: "development", Validation: &ValidationResult{Profile: "docs", Commit: commit}}
	if err = s.Complete(ctx, job.ExecutionID, job.LeaseToken, result); !errors.Is(err, ErrConflict) {
		t.Fatalf("rework reused rejected implementation at completion: %v", err)
	}
	must(s.RecordCICandidate(ctx, job.ExecutionID, job.LeaseToken, commit, "new-implementation"))
	complete("new-implementation")
}
