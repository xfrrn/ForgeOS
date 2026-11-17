package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLiveProcessCannotRegressOrOutliveLease(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{RepositoryID: 3, ID: 7, Number: 7}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.Claim(ctx, "node", t.TempDir(), true)
	if err != nil || job == nil || !job.ReportLiveProcess || !job.RecordModelOutput || !job.RecordToolOutput {
		t.Fatalf("expected live reporting job: %v", err)
	}
	now := time.Now().UTC()
	p := HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Started: now}
	post := func(proc HarnessProcess, token string, ok bool) {
		t.Helper()
		err := s.ObserveProcess(ctx, job.ExecutionID, ProcessReport{LeaseToken: token, Process: proc})
		if (ok && err != nil) || (!ok && !errors.Is(err, ErrConflict)) {
			t.Fatalf("report accepted=%v err=%v", ok, err)
		}
	}
	post(p, strings.Repeat("z", 64), false)
	post(p, job.LeaseToken, true)
	p.Session = "thread-a"
	post(p, job.LeaseToken, true)
	p.Output = &ModelOutput{Sequence: 1, Text: "Reading the guide.\n"}
	post(p, job.LeaseToken, true)
	for _, output := range []*ModelOutput{nil, {Sequence: 0, Text: "bad\n"}, {Sequence: 1, Text: "altered\n"},
		{Sequence: 2, Text: "token=secret\n"}, {Sequence: 2, Text: job.LeaseToken + "\n"}, {Sequence: 2, Text: "incomplete"}} {
		bad := p
		bad.Output = output
		post(bad, job.LeaseToken, false)
	}
	p.Output = &ModelOutput{Sequence: 2, Text: "Reading the guide.\nReady to edit.\n"}
	post(p, job.LeaseToken, true)
	first := ToolEvent{Seq: 1, Call: 1, At: now.Unix(), Tool: "command_execution", Phase: "started"}
	p.Tools = []ToolEvent{first}
	post(p, job.LeaseToken, true)
	for _, bad := range []ToolEvent{
		{Seq: 1, Call: 1, At: now.Unix(), Tool: "command_execution", Phase: "completed"},
		{Seq: 2, Call: 2, At: now.Unix(), Tool: "Read", Phase: "started"},
		{Seq: 2, Call: 2, At: 1 << 60, Tool: "command_execution", Phase: "started"},
	} {
		modified := p
		if bad.Seq == 1 {
			modified.Tools = []ToolEvent{bad}
		} else {
			modified.Tools = []ToolEvent{first, bad}
		}
		post(modified, job.LeaseToken, false)
	}
	p.Tools = append(p.Tools, ToolEvent{Seq: 2, Call: 1, At: now.Unix(), Tool: "command_execution", Phase: "completed"})
	p.ToolOutput = &ToolOutput{Seq: 2, Call: 1, Tool: "command_execution", Text: "Command finished.\n"}
	post(p, job.LeaseToken, true)
	for _, output := range []*ToolOutput{nil, {Seq: 2, Call: 1, Tool: "command_execution", Text: "rewritten\n"},
		{Seq: 1, Call: 1, Tool: "command_execution", Text: "early\n"}, {Seq: 3, Call: 1, Tool: "command_execution", Text: "future\n"},
		{Seq: 2, Call: 2, Tool: "command_execution", Text: "wrong call\n"}, {Seq: 2, Call: 1, Tool: "Read", Text: "wrong tool\n"},
		{Seq: 2, Call: 1, Tool: "command_execution", Text: job.LeaseToken + "\n"}} {
		bad := p
		bad.ToolOutput = output
		post(bad, job.LeaseToken, false)
	}
	stale := p
	stale.Tools = p.Tools[:1]
	post(stale, job.LeaseToken, false)
	changed := p
	changed.PID++
	post(changed, job.LeaseToken, false)
	changed = p
	changed.Session = ""
	post(changed, job.LeaseToken, false)
	changed = p
	changed.Harness = "claude"
	post(changed, job.LeaseToken, false)
	end := now.Add(time.Second)
	code := 0
	closed := p
	closed.Exited = &end
	closed.ExitCode = &code
	post(closed, job.LeaseToken, true)
	post(closed, job.LeaseToken, true)
	changed = closed
	changed.Output = &ModelOutput{Sequence: 3, Text: "Changed after exit.\n"}
	post(changed, job.LeaseToken, false)
	changed = closed
	changed.ToolOutput = &ToolOutput{Seq: 2, Call: 1, Tool: "command_execution", Text: "Changed after exit.\n"}
	post(changed, job.LeaseToken, false)
	post(p, job.LeaseToken, false)
	p.Instance++
	p.PID++
	post(p, job.LeaseToken, true)
	post(closed, job.LeaseToken, false)
	view, err := s.Task(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	observed := view.Execution.ObservedProcess
	if observed == nil || observed.PID != p.PID || observed.Exited != nil || observed.ObservedAt.IsZero() || observed.Output == nil || *observed.Output != *p.Output {
		t.Fatal(observed)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE executions SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", job.ExecutionID); err != nil {
		t.Fatal(err)
	}
	post(p, job.LeaseToken, false)
}

func TestFinalProcessOutputRejectsLeaseAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base, sha := strings.Repeat("a", 40), strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, ID: 1, Number: 1}, BaseCommit: base, Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.Claim(ctx, "node", t.TempDir(), true)
	if err != nil || j == nil {
		t.Fatal(j, err)
	}
	at, zero := time.Now().UTC(), 0
	h := ProcessHistory{{Instance: 1, PID: 7, Harness: "codex", Version: "1.2.3", Session: "thread", Started: at, Exited: &at, ExitCode: &zero, Output: &ModelOutput{Sequence: 1, Text: j.LeaseToken + "\n"}}}
	r := Result{Commit: sha, BaseCommit: base, SessionID: "thread", Harness: "codex", HarnessVersion: "1.2.3", Processes: &h, Validation: &ValidationResult{Commit: sha, Profile: "docs"}}
	if !h.matches(r) {
		t.Fatal("fixture must otherwise be a valid final report")
	}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, r); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted lease in final output", err)
	}
	h[0].Output = &ModelOutput{Sequence: 1, Text: "[redacted]\nFinished the guide.\n"}
	h[0].Tools = []ToolEvent{{Seq: 1, Call: 1, Tool: "command_execution", Phase: "completed", At: at.Unix()}}
	h[0].ToolOutput = &ToolOutput{Seq: 1, Call: 1, Tool: "command_execution", Text: j.LeaseToken + "\n"}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, r); !errors.Is(err, ErrConflict) {
		t.Fatal("accepted lease in tool output", err)
	}
	h[0].ToolOutput.Text = "Command finished.\n"
	observed := h[0]
	observed.Exited, observed.ExitCode = nil, nil
	if err = s.ObserveProcess(ctx, j.ExecutionID, ProcessReport{LeaseToken: j.LeaseToken, Process: observed}); err != nil {
		t.Fatal(err)
	}
	for _, output := range []*ModelOutput{nil, {Sequence: 1, Text: "Rewritten final message.\n"}} {
		bad := r
		processes := append(ProcessHistory(nil), h...)
		processes[0].Output = output
		bad.Processes = &processes
		if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, bad); !errors.Is(err, ErrConflict) {
			t.Fatal("rewrote observed output in final result", err)
		}
	}
	for _, output := range []*ToolOutput{nil, {Seq: 1, Call: 1, Tool: "command_execution", Text: "Rewritten final tool output.\n"}} {
		bad := r
		processes := append(ProcessHistory(nil), h...)
		processes[0].ToolOutput = output
		bad.Processes = &processes
		if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, bad); !errors.Is(err, ErrConflict) {
			t.Fatal("rewrote final tool output", err)
		}
	}
	for range 2 {
		if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Task(ctx, task.ID)
	if err != nil || got.Execution.Result.Processes == nil || (*got.Execution.Result.Processes)[0].Output == nil || *(*got.Execution.Result.Processes)[0].Output != *h[0].Output {
		t.Fatal(got, err)
	}
	if (*got.Execution.Result.Processes)[0].ToolOutput == nil || *(*got.Execution.Result.Processes)[0].ToolOutput != *h[0].ToolOutput {
		t.Fatal("tool history lost")
	}
}
