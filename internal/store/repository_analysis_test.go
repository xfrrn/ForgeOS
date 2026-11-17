package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"forgeos/internal/localrepo"
)

func TestRepositoryAnalysisPinsEvidenceAndLease(t *testing.T) {
	ctx := context.Background()
	db := isolatedDatabase(t)
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	p, err := s.CreateProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.Claim(ctx, "reader", root, false)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	if err = s.Complete(ctx, job.ExecutionID, job.LeaseToken, Result{Commit: sha}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveRepositoryInventory(ctx, task.ID, job.ExecutionID, sha, "", json.RawMessage("[]")); err != nil {
		t.Fatal(err)
	}
	input := localrepo.AnalysisInput{Sources: []localrepo.AnalysisSource{{Path: "README.md", Blob: sha, Basis: "git", Text: "Start with go run ./cmd/app"}}}
	if err = s.QueueAnalysis(ctx, task.ID, input); err != nil {
		t.Fatal(err)
	}
	if err = s.QueueAnalysis(ctx, task.ID, localrepo.AnalysisInput{}); err != nil {
		t.Fatal(err)
	}
	if j, err := s.ClaimAnalysis(ctx, p.ID+1, true); err != nil || j != nil {
		t.Fatal("wrong project", err)
	}
	first, err := s.ClaimAnalysis(ctx, p.ID, false)
	if err != nil || first == nil || first.LeaseUntil != nil || len(first.Input.Sources) != 1 {
		t.Fatal(err)
	}
	observe := func(report ManagementProcessReport, want bool) {
		t.Helper()
		e := s.ObserveManagementProcess(ctx, p.ID, report)
		if (want && e != nil) || (!want && !errors.Is(e, ErrConflict)) {
			t.Fatal("process observation", want, e)
		}
	}
	observation := ManagementProcessReport{AnalysisTask: task.ID, Lease: first.Lease, Node: "analysis-node", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Session: "expired-thread", Started: time.Now().UTC().Truncate(time.Second)}}
	observe(observation, true)
	bad := observation
	bad.TaskID = task.ID
	bad.Revision = 1
	observe(bad, false)
	if e := s.ObserveManagementProcess(ctx, p.ID+1, observation); !errors.Is(e, ErrConflict) {
		t.Fatal("cross project observation", e)
	}
	if err = s.SetModelBudgets(ctx, []ModelBudget{{Name: "account", Harness: "codex", Slots: 1, AgentRoles: []ModelBudgetRole{{Project: p.ID, Role: "repository_analysis"}}}}); err != nil {
		t.Fatal(err)
	}
	checkUsage := func(want int) {
		t.Helper()
		var used int
		if err := db.QueryRowContext(ctx, `SELECT reserved FROM model_budget_usage WHERE name='account'`).Scan(&used); err != nil || used != want {
			t.Fatalf("reserved %d, expected %d: %v", used, want, err)
		}
	}
	checkUsage(1)
	if j, err := s.ClaimAnalysis(ctx, p.ID, true); err != nil || j != nil {
		t.Fatal("concurrent claim", err)
	}
	if _, err = db.Exec("UPDATE repository_analyses SET lease_until=now()-interval '1 second' WHERE task_id=$1", task.ID); err != nil {
		t.Fatal(err)
	}
	checkUsage(0)
	if j, err := s.ClaimAnalysis(ctx, p.ID, false); err != nil || j != nil {
		t.Fatal("older worker received allocated account", j, err)
	}
	second, err := s.ClaimAnalysis(ctx, p.ID, true)
	if err != nil || second == nil || second.Lease == first.Lease || second.LeaseUntil == nil {
		t.Fatal(err)
	}
	r := AnalysisResult{Task: task.ID, Lease: first.Lease, Failure: "agent_run_failed"}
	if err = s.FinishAnalysis(ctx, p.ID, r); !errors.Is(err, ErrConflict) {
		t.Fatal("stale lease", err)
	}
	checkUsage(1)
	observe(observation, false)
	observation.Lease = second.Lease
	observation.Process.PID = 456
	observation.Process.Session = "failed-thread"
	observe(observation, true)
	r.Lease = second.Lease
	if e := s.FinishAnalysis(ctx, p.ID, r); !errors.Is(e, ErrConflict) {
		t.Fatal("failure accepted while process active", e)
	}
	end := observation.Process.Started.Add(time.Second)
	code := 15
	observation.Process.Exited = &end
	observation.Process.ExitCode = &code
	observe(observation, true)

	for range 2 {
		if err = s.FinishAnalysis(ctx, p.ID, r); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.RetryAnalysis(ctx, task.ID, 1); err != nil {
		t.Fatal(err)
	}
	checkUsage(0)
	if j, err := s.ClaimAnalysis(ctx, p.ID, true); err != nil || j != nil {
		t.Fatal("stale retry", err)
	}
	if err = s.RetryAnalysis(ctx, task.ID, 2); err != nil {
		t.Fatal(err)
	}
	third, err := s.ClaimAnalysis(ctx, p.ID, true)
	if err != nil || third == nil || !third.CaptureProfile {
		t.Fatal(err)
	}
	observation.Lease = third.Lease
	observation.Process.PID = 789
	observation.Process.Session = "session-qa"
	observation.Process.Exited = nil
	observation.Process.ExitCode = nil
	observation.Process.Output = &ModelOutput{Sequence: 1, Text: "Analyzing sources.\n"}
	observe(observation, true)
	fact := AnalysisFact{Kind: "start", Value: "invented command", Path: "README.md", Quote: "go run ./cmd/app"}
	r = AnalysisResult{Task: task.ID, Lease: third.Lease, Analysis: &RepositoryAnalysis{Session: "session-qa", Facts: []AnalysisFact{fact}}}
	if err = s.FinishAnalysis(ctx, p.ID, r); !errors.Is(err, ErrConflict) {
		t.Fatal("ungrounded command", err)
	}
	r.Analysis.Facts[0].Value = fact.Quote
	r.Analysis.Facts[0].Path = "stale.md"
	if err = s.FinishAnalysis(ctx, p.ID, r); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong source", err)
	}
	r.Analysis.Facts[0].Path = fact.Path
	if err = s.FinishAnalysis(ctx, p.ID+1, r); !errors.Is(err, ErrConflict) {
		t.Fatal("cross-project completion", err)
	}
	if !r.Analysis.Valid(third.Input) {
		t.Fatal("legacy analysis rejected")
	}
	profile := StructuredCodexProfile()
	r.Analysis.Profile = &profile
	profile.Permission = "on-request"
	if err = s.FinishAnalysis(ctx, p.ID, r); !errors.Is(err, ErrConflict) {
		t.Fatal("incorrect native permission", err)
	}
	profile = StructuredCodexProfile()
	if e := s.FinishAnalysis(ctx, p.ID, r); !errors.Is(e, ErrConflict) {
		t.Fatal("success accepted while process active", e)
	}
	code = 0
	observation.Process.Exited = &end
	observation.Process.ExitCode = &code
	observe(observation, true)
	r.Analysis.Session = "wrong-thread"
	if e := s.FinishAnalysis(ctx, p.ID, r); !errors.Is(e, ErrConflict) {
		t.Fatal("wrong session", e)
	}
	r.Analysis.Session = "session-qa"
	for range 2 {
		if err = s.FinishAnalysis(ctx, p.ID, r); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := s.RepositoryAnalysis(ctx, task.ID)
	checkUsage(0)
	if err != nil || strings.Contains(string(raw), "Start with") || !strings.Contains(string(raw), "session-qa") {
		t.Fatal(string(raw), err)
	}
	var receipt struct {
		Processes []ManagementProcess `json:"processes"`
		Analysis  RepositoryAnalysis  `json:"analysis"`
	}
	if err = json.Unmarshal(raw, &receipt); err != nil || receipt.Analysis.Profile == nil || !validStructuredProfile(receipt.Analysis.Profile) {
		t.Fatal("missing archived configuration", err)
	}
	if len(receipt.Processes) != 3 || receipt.Processes[0].Exited != nil || *receipt.Processes[1].ExitCode != 15 || receipt.Processes[2].Output.Text != "Analyzing sources.\n" {
		t.Fatal("attempt history lost")
	}
	observe(observation, false)
	r.Analysis.Profile = nil
	if err = s.FinishAnalysis(ctx, p.ID, r); !errors.Is(err, ErrConflict) {
		t.Fatal("replay removed archived permissions", err)
	}
}
