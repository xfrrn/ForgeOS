package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/acceptance"
	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestCandidatePlanRolesAndRevisions(t *testing.T) {
	ctx := context.Background()
	root, repo, base := issueTestRepo(t)
	data := store.New(testdb.Open(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	conflict := func(err error) {
		t.Helper()
		if !errors.Is(err, store.ErrConflict) {
			t.Fatal("expected conflict", err)
		}
	}
	must(data.Migrate(ctx))
	must(os.WriteFile(filepath.Join(repo, "guide.md"), []byte("Read the guide.\n"), 0600))
	for _, args := range [][]string{{"add", "guide.md"}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "document feature"}} {
		_, err := localrepo.Git(ctx, repo, args...)
		must(err)
	}
	out, err := localrepo.Git(ctx, repo, "rev-parse", "HEAD")
	must(err)
	sha := strings.TrimSpace(string(out))
	project, err := data.CreateProject(ctx, repo)
	must(err)
	task, err := data.ImportIssue(ctx, project.ID, store.IssueSpec{Issue: store.Issue{RepositoryID: 99, ID: 1, Number: 1, Title: "Read the guide."}, BaseCommit: base, Validation: "docs"})
	must(err)
	dev, err := data.Claim(ctx, "development", t.TempDir(), true)
	must(err)
	if dev == nil {
		t.Fatal("missing development job")
	}
	must(data.Complete(ctx, dev.ExecutionID, dev.LeaseToken, store.Result{BaseCommit: base, Commit: sha, SessionID: "dev", Validation: &store.ValidationResult{Profile: "docs", Commit: sha}}))
	request := func(previous int, criteria []string) error {
		_, err := data.RequestTestPlan(ctx, root, task.ID, dev.ExecutionID, sha, previous, criteria)
		return err
	}
	must(request(0, nil))
	must(request(0, nil))
	conflict(request(0, []string{"Different."}))
	approve := func() error {
		_, err := data.Review(ctx, task.ID, dev.ExecutionID, sha, "approve", "", false, "")
		return err
	}
	conflict(approve())
	claim := func(role string) *store.TestPlanJob {
		t.Helper()
		j, err := data.ClaimTestPlan(ctx, project.ID, role)
		must(err)
		if j == nil {
			t.Fatal("no job", role)
		}
		return j
	}
	old := claim("test")
	j, err := data.ClaimTestPlan(ctx, project.ID, "test")
	must(err)
	if j != nil {
		t.Fatal("concurrent lease")
	}
	must(request(1, nil))
	conflict(data.FinishTestPlan(ctx, project.ID, "test", store.TestPlanOutcome{ID: old.ID, Lease: old.Lease, Failed: true}))
	j = claim("test")
	d := acceptance.Draft{Cases: []acceptance.Case{{Criterion: "requirement/1", Playbook: "documentation", Objective: "Read", Preconditions: "Candidate ready", Environment: "local", Steps: []string{"Open guide.md"}, Expected: "Read the guide.", Evidence: []string{"File and line"}, Tester: "human"}}, Questions: []acceptance.Question{}}
	plan, err := j.Source.Basis.Bind("tester", j.Source.Library, d)
	must(err)
	r := store.TestPlanOutcome{ID: j.ID, Lease: j.Lease, Plan: &plan}
	conflict(data.FinishTestPlan(ctx, project.ID+1, "test", r))
	conflict(data.FinishTestPlan(ctx, project.ID, "management", r))
	plan.Session = "dev"
	conflict(data.FinishTestPlan(ctx, project.ID, "test", r))
	plan.Session = "tester"
	must(data.FinishTestPlan(ctx, project.ID, "test", r))
	must(data.FinishTestPlan(ctx, project.ID, "test", r))
	conflict(approve())
	j = claim("management")
	coverage, err := j.Plan.Coverage("manager", []acceptance.CoverageItem{{Criterion: "requirement/1", Cases: []string{"TC-001"}, State: "covered", Rationale: "File lines can be verified."}})
	must(err)
	v := store.TestPlanOutcome{ID: j.ID, Lease: j.Lease, Coverage: &coverage}
	original := coverage.Digest
	coverage.Digest = sha
	conflict(data.FinishTestPlan(ctx, project.ID, "management", v))
	coverage.Digest = original
	must(data.FinishTestPlan(ctx, project.ID, "management", v))
	must(data.FinishTestPlan(ctx, project.ID, "management", v))
	conflict(approve())
	caseResult := store.TestCaseResult{Execution: dev.ExecutionID, Revision: 2, Commit: sha, PlanDigest: original, Case: "TC-001", Result: "blocked", Operations: []string{"Open guide.md"}, Actual: "No review account available", Evidence: []string{"Test fixture observation"}}
	record := func(tester, actor string) error {
		_, err := data.RecordTestCase(ctx, task.ID, project.ID, tester, actor, caseResult)
		return err
	}
	conflict(record("runner", "node"))
	must(record("human", "owner"))
	must(record("human", "owner"))
	conflict(approve())
	caseResult.Result, caseResult.Actual = "passed", "Candidate contains Read the guide."
	conflict(record("human", "owner"))
	caseResult.Previous = 1
	caseResult.Commit = base
	conflict(record("human", "owner"))
	caseResult.Commit = sha
	must(record("human", "owner"))
	must(approve())
	must(record("human", "owner"))
}
