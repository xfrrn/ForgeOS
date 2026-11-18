package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNativeInputBindsOneLiveTurnAndClaimsOnlyOnce(t *testing.T) {
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
	project, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	_, err = s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{ID: 2, Number: 1, RepositoryID: 3}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	must(err)
	caps := WorkerCapabilities{SupportsCodex: true, SupportsCheckpoints: true, SupportsResources: true, SupportsModelPause: true}
	must(s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: "writer", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}))
	j, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), []int64{project.ID}, caps)
	must(err)
	if j == nil || !j.SupportsTurnInput {
		t.Fatal("missing live input negotiation")
	}
	id := j.ExecutionID
	ok, err := s.ReserveExecutionResource(ctx, id, ResourceRequest{Kind: "model", Step: 1, Lease: j.LeaseToken})
	must(err)
	if !ok {
		t.Fatal("missing model reservation")
	}
	proc := HarnessProcess{Instance: 3, PID: 42, Harness: "codex", Version: "1.2.3", Started: time.Now().UTC(), Session: "thread-1", Turn: "turn-2"}
	report := ProcessReport{LeaseToken: j.LeaseToken, Process: proc}
	input := NativeTurnInput{NativeTurn: NativeTurn{Instance: 3, Session: "thread-1", Turn: "turn-2"}, OwnerInstruction: OwnerInstruction{Key: "00000000-0000-4000-8000-000000000001", Body: "Use the supplied heading."}}
	must(s.ObserveProcess(ctx, id, report))
	conflict(s.QueueTurnInput(ctx, id, input))
	report.AcceptsTurnInput = true
	must(s.ObserveProcess(ctx, id, report))
	bad := input
	bad.Turn = "other"
	conflict(s.QueueTurnInput(ctx, id, bad))
	must(s.QueueTurnInput(ctx, id, input))
	must(s.QueueTurnInput(ctx, id, input))
	bad = input
	bad.Body = "Different"
	conflict(s.QueueTurnInput(ctx, id, bad))
	poll := TurnInputPoll{NativeTurn: input.NativeTurn, Lease: strings.Repeat("x", 64), Claim: strings.Repeat("a", 32)}
	item, err := s.PollTurnInput(ctx, id, poll)
	must(err)
	if item != nil {
		t.Fatal("wrong lease received an input")
	}
	poll.Lease = j.LeaseToken
	type claim struct {
		item *OwnerInstruction
		err  error
		poll TurnInputPoll
	}
	results := make(chan claim, 2)
	for _, key := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32)} {
		req := poll
		req.Claim = key
		go func() { item, err := s.PollTurnInput(ctx, id, req); results <- claim{item, err, req} }()
	}
	delivered := 0
	for range 2 {
		r := <-results
		must(r.err)
		if r.item != nil {
			delivered++
			if *r.item != input.OwnerInstruction {
				t.Fatal(r.item)
			}
			again, err := s.PollTurnInput(ctx, id, r.poll)
			must(err)
			if again == nil || *again != *r.item {
				t.Fatal("lost claim response did not replay", again)
			}
		}
	}
	if delivered != 1 {
		t.Fatalf("claimed %d times", delivered)
	}
	must(s.ControlModel(ctx, id, 3, false))
	bad = input
	bad.Key = "00000000-0000-4000-8000-000000000002"
	conflict(s.QueueTurnInput(ctx, id, bad))
	must(s.QueueTurnInput(ctx, id, input))
	poll.Key, poll.Outcome = input.Key, "accepted"
	for range 2 {
		_, err = s.PollTurnInput(ctx, id, poll)
		must(err)
	}
	poll.Outcome = "rejected"
	_, err = s.PollTurnInput(ctx, id, poll)
	conflict(err)
}
