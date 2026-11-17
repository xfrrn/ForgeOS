package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSharedModelBudget(t *testing.T) {
	ctx := context.Background()
	db := isolatedDatabase(t)
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project := func() int64 {
		t.Helper()
		p, err := s.CreateProject(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		spec := IssueSpec{Issue: Issue{RepositoryID: p.ID, ID: p.ID, Number: 1}, BaseCommit: strings.Repeat("1", 40), Validation: "docs"}
		if _, err = s.ImportIssue(ctx, p.ID, spec); err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	pool := ModelBudget{Name: "shared", Harness: "codex", Slots: 1, Workers: []string{"old", "a", "b"}}
	set := func(b ...ModelBudget) {
		t.Helper()
		if err := s.SetModelBudgets(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	used := func(name string) int {
		t.Helper()
		raw, err := s.ModelBudgets(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var rows []struct {
			Name     string
			Reserved int
		}
		if err = json.Unmarshal(raw, &rows); err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.Name == name {
				return row.Reserved
			}
		}
		t.Fatalf("missing budget %s", name)
		return -1
	}
	root := t.TempDir()
	project()
	old, err := s.Claim(ctx, "old", root, true)
	if err != nil || old == nil || old.ManageResources {
		t.Fatal("legacy execution missing", err)
	}
	set(pool)
	if used(pool.Name) != 1 {
		t.Fatal("active legacy model not counted")
	}
	project()
	if next, err := s.Claim(ctx, "a", root, true); err != nil || next != nil {
		t.Fatal("unmanaged node bypass", err)
	}
	caps := WorkerCapabilities{SupportsCodex: true, SupportsResources: true}
	var jobs []*Job
	names := map[*Job]string{}
	for _, name := range []string{"a", "b"} {
		if err := s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: name, OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}); err != nil {
			t.Fatal(err)
		}
		project()
		j, err := s.ClaimForProjects(ctx, name, root, nil, caps)
		if err != nil || j == nil || !j.ManageResources {
			t.Fatal("missing job", err)
		}
		jobs = append(jobs, j)
		names[j] = name
	}
	request := func(j *Job, step int64, kind string, release bool) (bool, error) {
		return s.ReserveExecutionResource(ctx, j.ExecutionID, ResourceRequest{Lease: j.LeaseToken, Step: step, Kind: kind, Release: release})
	}
	check := func(j *Job, step int64, kind string, release, expected bool) {
		t.Helper()
		if ok, err := request(j, step, kind, release); err != nil || ok != expected {
			t.Fatalf("%s step %d granted %t error %v", kind, step, ok, err)
		}
	}
	check(jobs[0], 1, "model", false, false)
	if err = s.Complete(ctx, old.ExecutionID, old.LeaseToken, Result{ExitCode: 1, Error: "settled"}); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		j   *Job
		ok  bool
		err error
	}
	ch := make(chan outcome, 2)
	for _, j := range jobs {
		go func() { ok, err := request(j, 1, "model", false); ch <- outcome{j, ok, err} }()
	}
	x, y := <-ch, <-ch
	if x.err != nil || y.err != nil || x.ok == y.ok || used(pool.Name) != 1 {
		t.Fatal("race", x.err, y.err)
	}
	if !x.ok {
		x, y = y, x
	}
	check(x.j, 1, "model", false, true)
	check(y.j, 1, "ci", false, true)
	check(y.j, 1, "ci", true, true)
	other := ModelBudget{Name: "different", Harness: "codex", Slots: 1, Workers: []string{names[x.j]}}
	if err := s.SetModelBudgets(ctx, []ModelBudget{pool, other}); !errors.Is(err, ErrConflict) {
		t.Fatal("overlap", err)
	}
	pool.Workers = []string{names[y.j]}
	set(pool, other)
	check(y.j, 2, "model", false, false)
	if used(other.Name) != 0 {
		t.Fatal("live charge moved to a different account")
	}
	if _, err = db.Exec(`UPDATE executions SET lease_until=now()-interval '1 second' WHERE id=$1`, x.j.ExecutionID); err != nil {
		t.Fatal(err)
	}
	check(y.j, 2, "model", false, false)
	if err = s.Abandon(ctx, x.j.ExecutionID); err != nil {
		t.Fatal(err)
	}
	check(y.j, 2, "model", false, true)
	check(y.j, 2, "model", true, true)
	if used(pool.Name) != 0 {
		t.Fatal("charge retained after release")
	}
}

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
