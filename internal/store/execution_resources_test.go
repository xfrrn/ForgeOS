package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecutionResourceReservations(t *testing.T) {
	ctx := context.Background()
	db := isolatedDatabase(t)
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		p, err := s.CreateProject(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		spec := IssueSpec{Issue: Issue{RepositoryID: p.ID, ID: p.ID, Number: 1}, BaseCommit: strings.Repeat("1", 40), Validation: "docs"}
		if _, err := s.ImportIssue(ctx, p.ID, spec); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	old, err := s.Claim(ctx, "node", root, true)
	if err != nil || old == nil || old.ManageResources {
		t.Fatal("legacy claim", err)
	}
	caps := WorkerCapabilities{SupportsCodex: true, SupportsResources: true}
	h := WorkerHeartbeat{Worker: "node", OS: "linux", Arch: "amd64", CPUs: 4, Slots: 3, ModelSlots: 1, CISlots: 1, Capabilities: caps}
	if err := s.RecordHeartbeat(ctx, h); err != nil {
		t.Fatal(err)
	}
	if j, err := s.Claim(ctx, h.Worker, root, true); err != nil || j != nil {
		t.Fatal("claim skipped required resource negotiation", err)
	}
	jobs := make([]*Job, 2)
	for i := range jobs {
		jobs[i], err = s.ClaimForProjects(ctx, h.Worker, root, nil, caps)
		if err != nil || jobs[i] == nil || !jobs[i].ManageResources {
			t.Fatal("missing managed execution", err)
		}
	}
	check := func(j *Job, step int64, kind string, release, granted bool, wantErr error) {
		t.Helper()
		ok, err := s.ReserveExecutionResource(ctx, j.ExecutionID, ResourceRequest{Lease: j.LeaseToken, Step: step, Kind: kind, Release: release})
		if ok != granted || !errors.Is(err, wantErr) {
			t.Fatalf("execution %d step %d kind %s release %t: granted %t error %v", j.ExecutionID, step, kind, release, ok, err)
		}
	}
	check(jobs[0], 1, "model", false, false, nil)
	check(jobs[1], 1, "ci", false, false, nil)
	if err := s.Complete(ctx, old.ExecutionID, old.LeaseToken, Result{ExitCode: 1, Error: "stopped"}); err != nil {
		t.Fatal(err)
	}
	type attempt struct {
		job     *Job
		granted bool
		err     error
	}
	results := make(chan attempt, 2)
	for _, job := range jobs {
		go func() {
			ok, err := s.ReserveExecutionResource(ctx, job.ExecutionID, ResourceRequest{Lease: job.LeaseToken, Step: 1, Kind: "model"})
			results <- attempt{job, ok, err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.granted == second.granted {
		t.Fatal("expected exactly one model reservation", first.err, second.err)
	}
	if !first.granted {
		first, second = second, first
	}
	a, b := first.job, second.job
	check(a, 1, "model", false, true, nil)
	check(a, 1, "model", true, true, nil)
	check(a, 1, "model", true, true, nil)
	check(a, 1, "model", false, false, ErrConflict)
	check(b, 1, "model", false, true, nil)
	check(a, 2, "ci", false, true, nil)
	check(a, 1, "model", true, false, ErrConflict)
	check(b, 1, "model", true, true, nil)
	check(b, 2, "ci", false, false, nil)
	if _, err := db.Exec("UPDATE executions SET lease_until=now()-interval '1 second' WHERE id=$1", a.ExecutionID); err != nil {
		t.Fatal(err)
	}
	check(a, 2, "ci", true, false, ErrConflict)
	check(b, 2, "ci", false, false, nil)
	if err := s.Abandon(ctx, a.ExecutionID); err != nil {
		t.Fatal(err)
	}
	check(b, 2, "ci", false, true, nil)
}
