package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"forgeos/internal/acceptance"
	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
)

func TestCandidatePlanRolesAndRevisions(t *testing.T) {
	t.Run("refreshed without rules", func(t *testing.T) { testCandidatePlanRoles(t, false, true, false) })
	t.Run("legacy", func(t *testing.T) { testCandidatePlanRoles(t, false, false, false) })
	t.Run("role processes", func(t *testing.T) { testCandidatePlanRoles(t, true, false, true) })
	t.Run("published rules", func(t *testing.T) { testCandidatePlanRoles(t, true, false, false) })
}

func testCandidatePlanRoles(t *testing.T, published, refresh, capture bool) {
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
	var revision json.RawMessage
	if published {
		revision, err = data.PublishGlobalRules(ctx, 0, "Review headings")
		must(err)
	}
	dev, err := data.ClaimForProjects(ctx, "development", t.TempDir(), []int64{project.ID}, store.WorkerCapabilities{SupportsCodex: true, SupportsGlobalRules: true})
	must(err)
	if dev == nil {
		t.Fatal("missing development job")
	}
	must(data.Complete(ctx, dev.ExecutionID, dev.LeaseToken, store.Result{BaseCommit: base, Commit: sha, SessionID: "dev", Validation: &store.ValidationResult{Profile: "docs", Commit: sha}}))
	if published {
		_, err = data.PublishGlobalRules(ctx, 1, "Future headings")
		must(err)
	}
	execution := dev.ExecutionID
	if refresh {
		_, err = data.Review(ctx, task.ID, execution, sha, "approve", "Checked", true, "main")
		must(err)
		delivery, err := data.ClaimDelivery(ctx, project.ID)
		must(err)
		if delivery == nil {
			t.Fatal("missing delivery")
		}
		delivery.State, delivery.PRNumber, delivery.PRURL = "WAITING", 1, "https://github.com/a/b/pull/1"
		must(data.SaveDelivery(ctx, *delivery))
		git := func(args ...string) string {
			t.Helper()
			out, err := localrepo.Git(ctx, repo, append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)...)
			must(err)
			return strings.TrimSpace(string(out))
		}
		git("checkout", "--detach", base)
		git("commit", "--allow-empty", "-qm", "advance target")
		base = git("rev-parse", "HEAD")
		git("cherry-pick", sha)
		refreshed := git("rev-parse", "HEAD")
		_, err = data.QueueRefreshedCandidate(ctx, task.ID, execution, sha, base, refreshed)
		must(err)
		ci, err := data.ClaimForProjects(ctx, "ci", t.TempDir(), []int64{project.ID}, store.WorkerCapabilities{SupportsValidation: true})
		must(err)
		if ci == nil {
			t.Fatal("missing CI job")
		}
		execution, sha = ci.ExecutionID, refreshed
		must(data.Complete(ctx, execution, ci.LeaseToken, store.Result{BaseCommit: base, Commit: sha, Validation: &store.ValidationResult{Profile: "docs", Commit: sha}}))
	}
	request := func(previous int, criteria []string) error {
		_, err := data.RequestTestPlan(ctx, root, task.ID, execution, sha, previous, criteria)
		return err
	}
	must(request(0, nil))
	must(request(0, nil))
	conflict(request(0, []string{"Different."}))
	approve := func() error {
		_, err := data.Review(ctx, task.ID, execution, sha, "approve", "", false, "")
		return err
	}
	conflict(approve())
	pool := store.ModelBudget{Name: "tests", Harness: "codex", Slots: 1, TestPlanRoles: []store.ModelBudgetRole{{Project: project.ID, Role: "test"}, {Project: project.ID, Role: "management"}}}
	// Configure after the first claim to exercise an upgrade with an in-flight turn.
	claim := func(role string) *store.TestPlanJob {
		t.Helper()
		if published {
			legacy, err := data.ClaimTestPlan(ctx, project.ID, role, false, false)
			must(err)
			if legacy != nil {
				t.Fatal("legacy role received candidate context", role)
			}
		}
		j, err := data.ClaimTestPlan(ctx, project.ID, role, published, true)
		must(err)
		if j == nil {
			t.Fatal("no job", role)
		}
		if !published {
			if j.Source.CandidateContext != nil || j.Source.Basis.CandidateContextSHA != "" {
				t.Fatal("unexpected rules for legacy role")
			}
			return j
		}
		var rules struct {
			Global json.RawMessage `json:"global_rules"`
		}
		must(json.Unmarshal(j.Source.CandidateContext, &rules))
		if string(rules.Global) != string(revision) {
			t.Fatal("missing implementation rules", role)
		}
		if len(j.Source.Basis.CandidateContextSHA) != 64 || strings.Contains(string(j.Source.CandidateContext), "Future headings") {
			t.Fatal("incorrect rule basis", role)
		}
		return j
	}
	process := func(job *store.TestPlanJob, role, session string) store.ManagementProcessReport {
		return store.ManagementProcessReport{TestPlanID: job.ID, TestPlanRole: role, Lease: job.Lease, Node: "test-node", Process: store.HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Session: session, Started: time.Now().UTC().Truncate(time.Second)}}
	}
	observe := func(p store.ManagementProcessReport) {
		t.Helper()
		must(data.ObserveManagementProcess(ctx, project.ID, p))
	}
	exit := func(p *store.ManagementProcessReport, code int) {
		t.Helper()
		end := p.Process.Started.Add(time.Second)
		p.Process.Exited = &end
		p.Process.ExitCode = &code
		observe(*p)
	}
	old := claim("test")
	oldProcess := process(old, "test", "old-tester")
	if capture {
		observe(oldProcess)
	}
	if published {
		must(data.SetModelBudgets(ctx, []store.ModelBudget{pool}))
		conflict(data.SetModelBudgets(ctx, []store.ModelBudget{pool, {Name: "other", Harness: "codex", Slots: 1, TestPlanRoles: pool.TestPlanRoles}}))
	}
	j, err := data.ClaimTestPlan(ctx, project.ID, "test", true, true)
	must(err)
	if j != nil {
		t.Fatal("concurrent lease")
	}
	must(request(1, nil))
	if published {
		pending, err := data.ClaimTestPlan(ctx, project.ID, "test", true, true)
		must(err)
		if pending != nil {
			t.Fatal("new revision started before the superseded turn released its slot")
		}
	}
	if capture {
		conflict(data.FinishTestPlan(ctx, project.ID, "test", store.TestPlanOutcome{ID: old.ID, Lease: old.Lease, Failed: true}))
		waiting, e := data.ClaimTestPlan(ctx, project.ID, "test", true, true)
		must(e)
		if waiting != nil {
			t.Fatal("running old process released slot")
		}
		exit(&oldProcess, 15)
	}
	conflict(data.FinishTestPlan(ctx, project.ID, "test", store.TestPlanOutcome{ID: old.ID, Lease: old.Lease, Failed: true}))
	j = claim("test")
	if published && j.LeaseUntil == nil {
		t.Fatal("missing budgeted plan deadline")
	}
	d := acceptance.Draft{Cases: []acceptance.Case{{Criterion: "requirement/1", Playbook: "documentation", Objective: "Read", Preconditions: "Candidate ready", Environment: "local", Steps: []string{"Open guide.md"}, Expected: "Read the guide.", Evidence: []string{"File and line"}, Tester: "human"}}, Questions: []acceptance.Question{}}
	plan, err := j.Source.Basis.Bind("tester", j.Source.Library, d)
	must(err)
	r := store.TestPlanOutcome{ID: j.ID, Lease: j.Lease, Plan: &plan}
	testProcess := process(j, "test", "tester")
	if capture {
		for _, change := range []func(*store.ManagementProcessReport){
			func(p *store.ManagementProcessReport) { p.TaskID = task.ID; p.Revision = 1 },
			func(p *store.ManagementProcessReport) { p.TestPlanRole = "management" },
			func(p *store.ManagementProcessReport) { p.TestPlanID = -1 },
			func(p *store.ManagementProcessReport) { p.CurationRole = "curator" },
			func(p *store.ManagementProcessReport) { p.Lease = "00000000-0000-0000-0000-000000000000" },
		} {
			bad := testProcess
			change(&bad)
			conflict(data.ObserveManagementProcess(ctx, project.ID, bad))
		}
		conflict(data.ObserveManagementProcess(ctx, project.ID+1, testProcess))
		observe(testProcess)
		conflict(data.FinishTestPlan(ctx, project.ID, "test", r))
		conflict(data.FinishTestPlan(ctx, project.ID, "test", store.TestPlanOutcome{ID: j.ID, Lease: j.Lease, Failed: true}))
		exit(&testProcess, 0)
		plan.Session = "wrong-tester"
		conflict(data.FinishTestPlan(ctx, project.ID, "test", r))
		plan.Session = "tester"
	}
	profile := store.StructuredCodexProfile()
	if !j.CaptureProfile {
		t.Fatal("missing profile negotiation")
	}
	if published {
		r.Profile = &profile
		profile.Permission = "on-request"
		conflict(data.FinishTestPlan(ctx, project.ID, "test", r))
		profile.Permission = store.CodexApproval
	}
	conflict(data.FinishTestPlan(ctx, project.ID+1, "test", r))
	conflict(data.FinishTestPlan(ctx, project.ID, "management", r))
	originalBasis := plan.Basis.CandidateContextSHA
	plan.Basis.CandidateContextSHA = strings.Repeat("0", 64)
	conflict(data.FinishTestPlan(ctx, project.ID, "test", r))
	plan.Basis.CandidateContextSHA = originalBasis
	plan.Session = "dev"
	conflict(data.FinishTestPlan(ctx, project.ID, "test", r))
	plan.Session = "tester"
	must(data.FinishTestPlan(ctx, project.ID, "test", r))
	must(data.FinishTestPlan(ctx, project.ID, "test", r))
	savedProfile := r.Profile
	r.Profile = &profile
	if published {
		r.Profile = nil
	}
	conflict(data.FinishTestPlan(ctx, project.ID, "test", r))
	r.Profile = savedProfile
	conflict(approve())
	if published {
		legacy, err := data.ClaimTestPlan(ctx, project.ID, "management", true, false)
		must(err)
		if legacy != nil {
			t.Fatal("legacy management worker acquired budgeted coverage job")
		}
	}
	j = claim("management")
	if published && j.LeaseUntil == nil {
		t.Fatal("missing budgeted coverage deadline")
	}
	coverage, err := j.Plan.Coverage("manager", []acceptance.CoverageItem{{Criterion: "requirement/1", Cases: []string{"TC-001"}, State: "covered", Rationale: "File lines can be verified."}})
	must(err)
	v := store.TestPlanOutcome{ID: j.ID, Lease: j.Lease, Coverage: &coverage}
	reviewProcess := process(j, "management", "manager")
	if capture {
		observe(reviewProcess)
		conflict(data.FinishTestPlan(ctx, project.ID, "management", v))
		exit(&reviewProcess, 0)
		coverage.Session = "wrong-manager"
		conflict(data.FinishTestPlan(ctx, project.ID, "management", v))
		coverage.Session = "manager"
	}
	if !j.CaptureProfile {
		t.Fatal("missing coverage profile negotiation")
	}
	if published {
		v.Profile = &profile
		profile.DisabledFeatures = nil
		conflict(data.FinishTestPlan(ctx, project.ID, "management", v))
		profile = store.StructuredCodexProfile()
	}
	original := coverage.Digest
	coverage.Digest = sha
	conflict(data.FinishTestPlan(ctx, project.ID, "management", v))
	coverage.Digest = original
	coverage.Session = "dev"
	conflict(data.FinishTestPlan(ctx, project.ID, "management", v))
	coverage.Session = "manager"
	must(data.FinishTestPlan(ctx, project.ID, "management", v))
	must(data.FinishTestPlan(ctx, project.ID, "management", v))
	v.Profile = &profile
	if published {
		v.Profile = nil
	}
	conflict(data.FinishTestPlan(ctx, project.ID, "management", v))
	receipt, err := data.TestPlanReceipt(ctx, task.ID, execution, 2)
	must(err)
	if capture {
		conflict(data.ObserveManagementProcess(ctx, project.ID, testProcess))
		conflict(data.ObserveManagementProcess(ctx, project.ID, reviewProcess))
		var histories struct {
			Test       []store.ManagementProcess `json:"test_processes"`
			Management []store.ManagementProcess `json:"management_processes"`
		}
		must(json.Unmarshal(receipt, &histories))
		if len(histories.Test) != 1 || len(histories.Management) != 1 || histories.Test[0].Session != "tester" || histories.Management[0].Session != "manager" {
			t.Fatal("role process histories", string(receipt))
		}
		oldReceipt, e := data.TestPlanReceipt(ctx, task.ID, execution, 1)
		must(e)
		must(json.Unmarshal(oldReceipt, &histories))
		if len(histories.Test) != 1 || *histories.Test[0].ExitCode != 15 || histories.Test[0].Session != "old-tester" {
			t.Fatal("lost superseded process")
		}
	}
	var fields map[string]json.RawMessage
	must(json.Unmarshal(receipt, &fields))
	for _, key := range []string{"test_profile", "management_profile"} {
		if (fields[key] != nil) != published {
			t.Fatalf("unexpected %s in receipt", key)
		}
		if published {
			var actual store.AgentProfile
			must(json.Unmarshal(fields[key], &actual))
			if !reflect.DeepEqual(actual, profile) {
				t.Fatalf("incorrect %s", key)
			}
		}
	}
	var receivedPlan acceptance.Plan
	must(json.Unmarshal(fields["plan"], &receivedPlan))
	receivedCoverage, err := receivedPlan.Coverage(coverage.Session, coverage.Items)
	must(err)
	if receivedCoverage.Digest != original {
		t.Fatal("profile changed fixed plan digest")
	}
	conflict(approve())
	caseResult := store.TestCaseResult{Execution: execution, Revision: 2, Commit: sha, PlanDigest: original, Case: "TC-001", Result: "blocked", Operations: []string{"Open guide.md"}, Actual: "No review account available", Evidence: []string{"Test fixture observation"}}
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
