package store

import (
	"context"
	"strings"
	"testing"

	"forgeos/internal/validation"
)

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
		if err != nil || (j != nil) != want || j != nil && j.TaskID != task.ID {
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
