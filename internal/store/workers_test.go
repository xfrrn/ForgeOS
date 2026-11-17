package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"forgeos/internal/validation"
)

func TestWorkerWorkspaceUsesNodePaths(t *testing.T) {
	for root, want := range map[string]string{
		"/var/work/":          "/var/work/execution-1",
		"/":                   "/execution-1",
		`C:\work`:             `C:\work\execution-1`,
		"C:/":                 `C:\execution-1`,
		`\\server\share\work`: `\\server\share\work\execution-1`,
	} {
		if !ValidWorkspaceRoot(root) || workerWorkspace(root, "execution-1") != want {
			t.Errorf("workspace under %q: expected %q", root, want)
		}
	}
	for _, root := range []string{"", "work", "C:work", `\relative`, "\x00/work"} {
		if ValidWorkspaceRoot(root) {
			t.Errorf("accepted workspace root %q", root)
		}
	}
}

func TestExecutionLeaseRenewal(t *testing.T) {
	ctx := context.Background()
	db := isolatedDatabase(t)
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.CreateTask(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx, "node", t.TempDir())
	if err != nil || j == nil || j.LeaseSeconds != 120 {
		t.Fatal("expected two-minute Git lease", err)
	}
	if err := s.RenewExecutionLease(ctx, j.ExecutionID, "wrong"); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted wrong token", err)
	}
	var shortened time.Time
	if err := db.QueryRow("UPDATE executions SET lease_until=clock_timestamp()+interval '5 seconds' WHERE id=$1 RETURNING lease_until", j.ExecutionID).Scan(&shortened); err != nil {
		t.Fatal(err)
	}
	if err := s.RenewExecutionLease(ctx, j.ExecutionID, j.LeaseToken); err != nil {
		t.Fatal(err)
	}
	got, err := s.Task(ctx, task.ID)
	if err != nil || got.Execution == nil || got.Execution.LeaseUntil.Sub(shortened) < 114*time.Second {
		t.Fatal("lease deadline not extended", err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE executions SET lease_until=clock_timestamp()+interval '100 milliseconds' WHERE id=$1", j.ExecutionID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.RenewExecutionLease(ctx, j.ExecutionID, j.LeaseToken) }()
	time.Sleep(160 * time.Millisecond)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrConflict) {
		t.Fatal("renewed expired lease", err)
	}
}

func TestRegisteredHarnessRequiredForClaim(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{
		Issue: Issue{RepositoryID: 1, ID: 1, Number: 1}, BaseCommit: strings.Repeat("a", 40),
		Validation: "auto", Policy: &validation.Policy{}, Harness: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	h := WorkerHeartbeat{Worker: "node", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1}
	request := WorkerCapabilities{SupportsClaude: true, SupportsRiskPolicy: true, SupportsDeclaredChecks: true}
	h.Capabilities = request
	h.Capabilities.SupportsClaude = false
	root := t.TempDir()
	check := func(want bool) {
		t.Helper()
		if err := s.RecordHeartbeat(ctx, h); err != nil {
			t.Fatal(err)
		}
		j, err := s.ClaimForProjects(ctx, h.Worker, root, []int64{p.ID}, request)
		if err != nil || (j != nil) != want || j != nil && (j.TaskID != task.ID || j.LeaseSeconds != 1500) {
			t.Fatalf("want claim %v, got %v, error %v", want, j, err)
		}
	}
	check(false)
	h.Capabilities.SupportsClaude = true
	h.Capabilities.SupportsDeclaredChecks = false
	check(false)
	h.Capabilities.SupportsDeclaredChecks = true
	request.SupportsRiskPolicy = false
	check(false)
	request.SupportsRiskPolicy = true
	check(true)
}

func TestAdapterAgentProfiles(t *testing.T) {
	caps := WorkerCapabilities{SupportsCodex: true, SupportsClaude: true, SupportsPlanner: true}
	h := WorkerHeartbeat{Worker: "node", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, Capabilities: caps, Adapters: map[string]AdapterProbe{}}
	for _, name := range []string{"codex", "claude"} {
		h.Adapters[name] = AdapterProbe{Version: "1.2.3", Operations: []string{"start"}, Profiles: caps.Profiles(name)}
	}
	if !h.Valid() {
		t.Fatal("enabled role profiles rejected")
	}
	h.Capabilities.SupportsPlanner = false
	if h.Valid() {
		t.Fatal("disabled planner reported")
	}
	delete(h.Adapters["codex"].Profiles, "planner")
	if !h.Valid() {
		t.Fatal("implementation-only node rejected")
	}
	(*h.Adapters["claude"].Profiles["implementation"].Tools)[0] = "Bash"
	if h.Valid() {
		t.Fatal("unexpected tool accepted in profile v1")
	}
}

func TestConcurrentNodeReservations(t *testing.T) {
	ctx := context.Background()
	db := isolatedDatabase(t)
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for range 6 {
		p, err := s.CreateProject(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.CreateTask(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	h := WorkerHeartbeat{Worker: "node", OS: "linux", Arch: "amd64", CPUs: 4, Slots: 2}
	if err := s.RecordHeartbeat(ctx, h); err != nil {
		t.Fatal(err)
	}
	type result struct {
		job *Job
		err error
	}
	claims := make(chan result, 8)
	for range cap(claims) {
		go func() { j, err := s.Claim(ctx, h.Worker, root); claims <- result{j, err} }()
	}
	var active []*Job
	for range cap(claims) {
		r := <-claims
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.job != nil {
			active = append(active, r.job)
		}
	}
	if len(active) != 2 {
		t.Fatalf("reserved %d of two slots", len(active))
	}
	h.Slots = 1
	if err := s.RecordHeartbeat(ctx, h); err != nil {
		t.Fatal(err)
	}
	finish := func(j *Job) {
		t.Helper()
		if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{Commit: strings.Repeat("1", 40)}); err != nil {
			t.Fatal(err)
		}
	}
	finish(active[0])
	if j, err := s.Claim(ctx, h.Worker, root); err != nil || j != nil {
		t.Fatal("reduced node capacity was exceeded", j, err)
	}
	finish(active[1])
	if _, err := db.ExecContext(ctx, "UPDATE worker_nodes SET last_seen=now()-interval '1 minute'"); err != nil {
		t.Fatal(err)
	}
	if j, err := s.Claim(ctx, h.Worker, root); err != nil || j != nil {
		t.Fatal("stale node reserved work", j, err)
	}
	if err := s.RecordHeartbeat(ctx, h); err != nil {
		t.Fatal(err)
	}
	if j, err := s.Claim(ctx, h.Worker, root); err != nil || j == nil {
		t.Fatal("reconnected node could not reserve", err)
	}
}
