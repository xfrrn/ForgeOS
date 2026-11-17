package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPlannerPauseRequiresUpdatedWorker(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, plannerPause := range []bool{false, true} {
		p, err := s.CreateProject(ctx, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.ImportIssue(ctx, p.ID, IssueSpec{Mode: "plan", Issue: Issue{ID: p.ID, Number: 1, RepositoryID: p.ID}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
		if err != nil {
			t.Fatal(err)
		}
		worker := "old-planner"
		if plannerPause {
			worker = "updated-planner"
		}
		caps := WorkerCapabilities{SupportsCodex: true, SupportsPlanner: true, SupportsCheckpoints: true, SupportsResources: true, SupportsModelPause: true, SupportsPlannerPause: plannerPause}
		if err := s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: worker, OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}); err != nil {
			t.Fatal(err)
		}
		j, err := s.ClaimForProjects(ctx, worker, t.TempDir(), []int64{p.ID}, caps)
		if err != nil || j == nil || j.ModelPause != plannerPause || j.RuntimeQuestions {
			t.Fatalf("planner pause capability %v: job=%v, error=%v", plannerPause, j != nil, err)
		}
	}
}

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
	poll := PausePoll{Lease: j.LeaseToken, Step: 1, Session: proc.Session, Trees: &trees, ResumeInstance: strings.Repeat("c", 64)}
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
	_, err = s.PollModelPause(ctx, id, poll)
	must(err)
	competitor := poll
	competitor.ResumeInstance = strings.Repeat("d", 64)
	if _, err := s.PollModelPause(ctx, id, competitor); !errors.Is(err, ErrModelResumeClaimed) {
		t.Fatalf("another process claimed the same resume: %v", err)
	}
	competitor.ResumeInstance = ""
	_, err = s.PollModelPause(ctx, id, competitor)
	conflict(err)
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
	proc.Instance++
	proc.PID++
	must(s.ObserveProcess(ctx, id, ProcessReport{LeaseToken: j.LeaseToken, Process: proc}))
	must(s.ControlModel(ctx, id, proc.Instance, false))
	tx, err = s.Task(ctx, task.ID)
	must(err)
	history := tx.Execution.ModelPauses
	if len(history) != 2 || history[0].Instance != 4 || history[0].Resumed == nil || history[0].Worktree != trees.Worktree ||
		history[1].Instance != proc.Instance || history[1].Parked != nil || tx.Execution.ModelPause.Instance != proc.Instance {
		t.Fatalf("unexpected pause history: %+v", history)
	}
}
