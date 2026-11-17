package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestModelPauseBindsProcessWorkspaceAndResourceStep(t *testing.T) {
	ctx := context.Background()
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
	p, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 2, Number: 1, RepositoryID: 3}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	must(err)
	caps := WorkerCapabilities{SupportsCodex: true, SupportsCheckpoints: true, SupportsResources: true, SupportsModelPause: true}
	must(s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: "writer", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}))
	j, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), []int64{p.ID}, caps)
	must(err)
	if j == nil || !j.ModelPause {
		t.Fatal("missing negotiated model control")
	}
	id := j.ExecutionID
	phase := ResourceRequest{Kind: "model", Step: 1, Lease: j.LeaseToken}
	ok, err := s.ReserveExecutionResource(ctx, id, phase)
	must(err)
	if !ok {
		t.Fatal("missing model slot")
	}
	proc := HarnessProcess{Instance: 4, PID: 101, Harness: "codex", Version: "1.2.3", Started: time.Now().UTC(), Session: "thread-1"}
	must(s.ObserveProcess(ctx, id, ProcessReport{LeaseToken: j.LeaseToken, Process: proc}))
	conflict(s.ControlModel(ctx, id, 5, false))
	must(s.ControlModel(ctx, id, 4, false))
	must(s.ControlModel(ctx, id, 4, false))
	conflict(s.ControlModel(ctx, id, 4, true))
	trees := WorktreeTrees{Index: strings.Repeat("b", 40), Worktree: strings.Repeat("c", 40)}
	poll := PausePoll{Lease: j.LeaseToken, Step: 1, Session: proc.Session, Trees: &trees}
	_, err = s.PollModelPause(ctx, id, poll)
	conflict(err)
	phase.Release = true
	_, err = s.ReserveExecutionResource(ctx, id, phase)
	must(err)
	bad := poll
	bad.Lease = strings.Repeat("f", 64)
	_, err = s.PollModelPause(ctx, id, bad)
	conflict(err)
	pause, err := s.PollModelPause(ctx, id, poll)
	must(err)
	if pause.Parked == nil || pause.Resumed != nil || pause.Worktree != trees.Worktree {
		t.Fatal(pause)
	}
	_, err = s.PollModelPause(ctx, id, poll)
	must(err)
	tx, err := s.Task(ctx, task.ID)
	must(err)
	if tx.Status != "PAUSED" || tx.Execution.ModelPause.Instance != 4 {
		t.Fatal(tx)
	}
	bad = poll
	bad.Trees = &WorktreeTrees{Index: trees.Worktree, Worktree: trees.Index}
	_, err = s.PollModelPause(ctx, id, bad)
	conflict(err)
	must(s.ControlModel(ctx, id, 4, true))
	must(s.ControlModel(ctx, id, 4, true))
	pause, err = s.PollModelPause(ctx, id, poll)
	must(err)
	if pause.Resumed == nil {
		t.Fatal(pause)
	}
	phase.Release = false
	phase.Step++
	ok, err = s.ReserveExecutionResource(ctx, id, phase)
	must(err)
	if !ok {
		t.Fatal("resumed model slot unavailable")
	}
	must(s.ObserveProcess(ctx, id, ProcessReport{LeaseToken: j.LeaseToken, Process: proc}))
	must(s.ControlModel(ctx, id, proc.Instance, false))
	pause, err = s.PollModelPause(ctx, id, PausePoll{Lease: j.LeaseToken, Step: phase.Step})
	must(err)
	if pause != nil {
		t.Fatal("replayed process paused the next model turn")
	}
	tx, err = s.Task(ctx, task.ID)
	must(err)
	if tx.Status != "RUNNING" || tx.Execution.PausableInstance != 0 {
		t.Fatal("stale process is pausable")
	}
}
