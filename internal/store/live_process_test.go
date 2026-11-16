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
	if err != nil || job == nil || !job.ReportLiveProcess {
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
	post(p, job.LeaseToken, true)
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
	if observed == nil || observed.PID != p.PID || observed.Exited != nil || observed.ObservedAt.IsZero() {
		t.Fatal(observed)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE executions SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1", job.ExecutionID); err != nil {
		t.Fatal(err)
	}
	post(p, job.LeaseToken, false)
}
