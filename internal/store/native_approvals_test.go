package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNativeCommandDecisionAndConfirmation(t *testing.T) {
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
	caps := WorkerCapabilities{SupportsCodex: true, SupportsCheckpoints: true, SupportsResources: true, SupportsModelPause: true, CommandApprovalProject: project.ID}
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
	turn := NativeTurn{Instance: proc.Instance, Session: proc.Session, Turn: proc.Turn}
	report.AcceptsTurnInput = true
	must(s.ObserveProcess(ctx, id, report))
	if !j.NativeApprovals {
		t.Fatal("registered project did not negotiate command approval")
	}
	r := NativeCommandRequest{NativeTurn: turn, Lease: j.LeaseToken, Item: "exec-1", Command: "fixed-command", Cwd: j.Workspace}
	d, err := s.PollNativeCommand(ctx, id, r)
	if err != nil || d != "" {
		t.Fatal(d, err)
	}
	var requestID int64
	must(s.db.QueryRowContext(ctx, "SELECT id FROM native_command_approvals WHERE execution_id=$1", id).Scan(&requestID))
	for _, change := range []func(*NativeCommandRequest){
		func(r *NativeCommandRequest) { r.Lease = strings.Repeat("0", 64) }, func(r *NativeCommandRequest) { r.Cwd += "x" },
		func(r *NativeCommandRequest) { r.Command += "x" }, func(r *NativeCommandRequest) { r.Turn += "x" },
	} {
		other := r
		change(&other)
		_, err = s.PollNativeCommand(ctx, id, other)
		conflict(err)
	}
	conflict(s.DecideNativeCommand(ctx, requestID, "acceptForSession"))
	must(s.DecideNativeCommand(ctx, requestID, "accept"))
	must(s.DecideNativeCommand(ctx, requestID, "accept"))
	conflict(s.DecideNativeCommand(ctx, requestID, "decline"))
	d, err = s.PollNativeCommand(ctx, id, r)
	if err != nil || d != "accept" {
		t.Fatal(d, err)
	}
	receipt := NativeCommandReceipt{NativeTurn: r.NativeTurn, Lease: r.Lease, Item: r.Item, Status: "completed"}
	must(s.ConfirmNativeCommand(ctx, id, receipt))
	must(s.ConfirmNativeCommand(ctx, id, receipt))
	receipt.Status = "failed"
	conflict(s.ConfirmNativeCommand(ctx, id, receipt))
	receipt.Item = "exec-2"
	conflict(s.ConfirmNativeCommand(ctx, id, receipt))
	r.Item = "exec-2"
	d, err = s.PollNativeCommand(ctx, id, r)
	if err != nil || d != "" {
		t.Fatal("decision reused", d, err)
	}
	_, err = s.db.ExecContext(ctx, "UPDATE native_command_approvals SET valid_until=clock_timestamp() WHERE execution_id=$1", id)
	must(err)
	must(s.db.QueryRowContext(ctx, "SELECT id FROM native_command_approvals WHERE item_id='exec-2'").Scan(&requestID))
	conflict(s.DecideNativeCommand(ctx, requestID, "accept"))
	r.Item = "exec-1"
	_, err = s.PollNativeCommand(ctx, id, r)
	conflict(err)
}
