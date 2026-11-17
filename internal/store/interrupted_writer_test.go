package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestInterruptedWriterRequiresBothOriginalExitReceipts(t *testing.T) {
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
	base := strings.Repeat("a", 40)
	_, err = s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, Number: 2, ID: 3}, BaseCommit: base, Validation: "docs"})
	must(err)
	caps := WorkerCapabilities{SupportsCodex: true, SupportsCheckpoints: true, SupportsResources: true, SupportsModelPause: true}
	must(s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: "writer", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}))
	job, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), nil, caps)
	must(err)
	if job == nil || !job.InterruptedHandoff {
		t.Fatal("missing negotiated handoff")
	}
	seq := 0
	check := func(request CheckpointRequest) error {
		request.Sequence, request.Lease = &seq, job.LeaseToken
		_, err := s.WorkerCheckpoint(ctx, job.ExecutionID, request, true)
		return err
	}
	must(check(CheckpointRequest{}))
	at := time.Now().UTC().Truncate(time.Second)
	crash, one, zero := 9, 1, 0
	src := HarnessProcess{Instance: 4, PID: 10, Harness: "codex", Version: "1.2.3", Session: "first", Started: at, Exited: &at, ExitCode: &crash}
	missing := HarnessProcess{Instance: 6, PID: 11, Harness: "codex", Version: src.Version, Started: at, Exited: &at, ExitCode: &one}
	proof := InterruptedWriterTurn{Source: src, Missing: missing, Trees: WorktreeTrees{Index: base, Worktree: strings.Repeat("b", 40)}}
	request := CheckpointRequest{Interrupted: &proof}
	conflict(check(request))
	phase := ResourceRequest{Lease: job.LeaseToken, Kind: "model", Step: 1}
	ok, err := s.ReserveExecutionResource(ctx, job.ExecutionID, phase)
	must(err)
	if !ok {
		t.Fatal("missing model slot")
	}
	must(s.ObserveProcess(ctx, job.ExecutionID, ProcessReport{LeaseToken: job.LeaseToken, Process: src}))
	proof.Missing.ExitCode = &zero
	conflict(check(request))
	proof.Missing = missing
	proof.Source.PID++
	conflict(check(request))
	proof.Source = src
	must(check(request))
	must(check(request))
	proof.Trees.Worktree = base
	conflict(check(request))
	link := SessionHandoff{From: src.Session, Session: "next", SourceInstance: src.Instance, Instance: missing.Instance}
	confirm := CheckpointRequest{Session: link.Session, Handoffs: []SessionHandoff{link}}
	conflict(check(confirm))
	confirm.Handoffs[0].Instance++
	must(check(confirm))
	must(check(confirm))
	phase.Release = true
	_, err = s.ReserveExecutionResource(ctx, job.ExecutionID, phase)
	must(err)
	next := HarnessProcess{Instance: missing.Instance + 1, PID: 12, Harness: "codex", Version: src.Version, Session: "next", HandoffFrom: "first", Started: at, Exited: &at, ExitCode: &zero}
	history := ProcessHistory{src, missing, next}
	result := Result{BaseCommit: base, ExitCode: 1, Error: "later failure", Harness: "codex", HarnessVersion: src.Version, SessionID: next.Session, Processes: &history}
	history[1].PID++
	conflict(s.Complete(ctx, job.ExecutionID, job.LeaseToken, result))
	history[1] = missing
	must(s.Complete(ctx, job.ExecutionID, job.LeaseToken, result))
}
