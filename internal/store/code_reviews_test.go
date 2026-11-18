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
	t.Run("legacy", func(t *testing.T) { testCodeReviewGate(t, false) })
	t.Run("process", func(t *testing.T) { testCodeReviewGate(t, true) })
}

func testCodeReviewGate(t *testing.T, capture bool) {
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
		j, err := s.ClaimCodeReview(ctx, projectID, false, false)
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
	budget := ModelBudget{Name: "review-account", Harness: "claude", Slots: 1, Workers: []string{"dev-claude"}, ReviewerProjects: []int64{project.ID}}
	must(s.SetModelBudgets(ctx, []ModelBudget{budget}))
	other, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	_, err = s.ImportIssue(ctx, other.ID, IssueSpec{Issue: Issue{RepositoryID: 9, ID: 10, Number: 1}, Harness: "claude", BaseCommit: base, Validation: "docs"})
	must(err)
	caps := WorkerCapabilities{SupportsClaude: true, SupportsResources: true}
	must(s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: "dev-claude", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}))
	dev, err := s.ClaimForProjects(ctx, "dev-claude", t.TempDir(), []int64{other.ID}, caps)
	must(err)
	if dev == nil {
		t.Fatal("Claude job missing")
	}
	phase := func(step int64, release, want bool) {
		t.Helper()
		ok, err := s.ReserveExecutionResource(ctx, dev.ExecutionID, ResourceRequest{Lease: dev.LeaseToken, Step: step, Kind: "model", Release: release})
		must(err)
		if ok != want {
			t.Fatalf("model reservation %d: got %t", step, ok)
		}
	}
	phase(1, false, false)
	process := func(j *ReviewJob, session string) ManagementProcessReport {
		return ManagementProcessReport{ReviewerExecution: ex, Lease: j.Lease, Node: "review-node", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "claude", Version: "1.2.3", Session: session, Started: time.Now().UTC().Truncate(time.Second)}}
	}
	observe := func(p ManagementProcessReport) { t.Helper(); must(s.ObserveManagementProcess(ctx, project.ID, p)) }
	exit := func(p *ManagementProcessReport, code int) {
		t.Helper()
		end := p.Process.Started.Add(time.Second)
		p.Process.Exited = &end
		p.Process.ExitCode = &code
		observe(*p)
	}
	failedProcess := process(j, "failed-review")
	if capture {
		for _, change := range []func(*ManagementProcessReport){
			func(p *ManagementProcessReport) { p.ReviewerExecution = -1 },
			func(p *ManagementProcessReport) { p.TaskID = task.ID; p.Revision = 1 },
			func(p *ManagementProcessReport) { p.TestPlanRole = "management" },
			func(p *ManagementProcessReport) { p.CurationRole = "curator" },
			func(p *ManagementProcessReport) { p.Process.Harness = "codex" },
			func(p *ManagementProcessReport) { p.Lease = strings.Repeat("x", 36) },
		} {
			bad := failedProcess
			change(&bad)
			conflict(s.ObserveManagementProcess(ctx, project.ID, bad))
		}
		conflict(s.ObserveManagementProcess(ctx, project.ID+1, failedProcess))
		observe(failedProcess)
		conflict(s.FailCodeReview(ctx, project.ID, ex, j.Lease))
		phase(1, false, false)
		exit(&failedProcess, 15)
		bad := review.Report{Base: base, Commit: commit, Session: "failed-review"}
		conflict(s.CompleteCodeReview(ctx, project.ID, ex, j.Lease, bad))
	}
	must(s.FailCodeReview(ctx, project.ID, ex, j.Lease))
	if capture {
		must(s.FailCodeReview(ctx, project.ID, ex, j.Lease))
		conflict(s.ObserveManagementProcess(ctx, project.ID, failedProcess))
	}
	checkState("failed")
	if claim(project.ID) != nil {
		t.Fatal("failed review retried without request")
	}
	conflict(approve())
	must(s.RequestCodeReview(ctx, task.ID, ex, commit))
	checkState("pending")
	phase(1, false, true)
	if blocked, err := s.ClaimCodeReview(ctx, project.ID, false, true); err != nil || blocked != nil {
		t.Fatal("shared account exceeded", err)
	}
	phase(1, true, true)
	if claim(project.ID) != nil {
		t.Fatal("reviewer without deadline support claimed budgeted job")
	}
	retry, err := s.ClaimCodeReview(ctx, project.ID, false, true)
	must(err)
	if retry == nil || retry.Lease == j.Lease || retry.LeaseUntil == nil {
		t.Fatal("missing new lease")
	}
	conflict(s.FailCodeReview(ctx, project.ID, ex, j.Lease))
	phase(2, false, false)
	_, err = s.db.ExecContext(ctx, `UPDATE code_reviews SET lease_until=now()-interval '1 second' WHERE execution_id=$1`, ex)
	must(err)
	conflict(s.FailCodeReview(ctx, project.ID, ex, retry.Lease))
	phase(2, false, true)
	phase(2, true, true)
	replaced, err := s.ClaimCodeReview(ctx, project.ID, false, true)
	must(err)
	if replaced == nil || replaced.Lease == retry.Lease {
		t.Fatal("expired review did not receive a fresh lease")
	}
	retry = replaced
	report := review.Report{Base: base, Commit: commit, Session: "dev-session", Summary: "Review complete.", Findings: []review.Finding{{ID: "R1", EvidenceVerified: false}}}
	complete := func(projectID int64, lease string) error {
		return s.CompleteCodeReview(ctx, projectID, ex, lease, report)
	}
	conflict(complete(project.ID, retry.Lease))
	report.Session = "review-session"
	if capture {
		p := process(retry, report.Session)
		observe(p)
		conflict(complete(project.ID, retry.Lease))
		conflict(s.FailCodeReview(ctx, project.ID, ex, retry.Lease))
		exit(&p, 0)
		report.Session = "wrong-review"
		conflict(complete(project.ID, retry.Lease))
		report.Session = "review-session"
	}
	conflict(complete(project.ID+1, retry.Lease))
	conflict(complete(project.ID, j.Lease))
	must(complete(project.ID, retry.Lease))
	must(complete(project.ID, retry.Lease))
	phase(3, false, true)
	phase(3, true, true)
	report.Summary = "Modified"
	checkState("needs_check")
	conflict(complete(project.ID, retry.Lease))
	conflict(approve())
	conflict(s.CheckReviewFindings(ctx, task.ID, ex, base, "wrong candidate"))
	conflict(s.CheckReviewFindings(ctx, task.ID, ex, commit, " "))
	must(s.CheckReviewFindings(ctx, task.ID, ex, commit, "Read the referenced line and confirmed the proposed finding does not apply."))
	must(approve())
	checkState("clear")
	must(s.SetCodeReviewPolicy(ctx, project.ID, false))
	must(approve()) // A replay retains the policy used for the original approval.
	approved, err := s.Task(ctx, task.ID)
	must(err)
	if len(approved.Reviews) != 1 || approved.Reviews[0].ApprovalBasis == nil {
		t.Fatal("missing original approval basis")
	}
	var basis map[string]bool
	must(json.Unmarshal(*approved.Reviews[0].ApprovalBasis, &basis))
	if !basis["project_review_required"] || !basis["code_review_present"] {
		t.Fatalf("changed approval basis after policy update: %v", basis)
	}
	receipt, err := s.ReviewReceipt(ctx, task.ID, ex)
	must(err)
	if capture {
		var r struct {
			Processes []ManagementProcess `json:"processes"`
		}
		must(json.Unmarshal(receipt, &r))
		if len(r.Processes) != 2 || *r.Processes[0].ExitCode != 15 || *r.Processes[1].ExitCode != 0 || r.Processes[0].Session != "failed-review" || r.Processes[1].Session != "review-session" {
			t.Fatal("lost review attempt histories")
		}
	}
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
	j, err := s.ClaimCodeReview(ctx, p.ID, false, false)
	must(err)
	if j != nil {
		t.Fatal("legacy reviewer received rules it cannot use")
	}
	j, err = s.ClaimCodeReview(ctx, p.ID, true, false)
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
