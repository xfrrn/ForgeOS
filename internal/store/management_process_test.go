package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManagementProcessBindsLeaseRevisionAndFinalReview(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, "management-output")
	must(err)
	task, err := s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{ID: 1, Number: 1, RepositoryID: 9}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	must(err)
	plan := Plan{Objective: "Guide", Milestones: []Milestone{{Name: "Guide", Items: []PlanItem{{Key: "guide", Objective: "Guide", Scope: "README", Deliverables: "README", Acceptance: "Read guide"}}}}}
	_, err = s.SavePlan(ctx, task.ID, 0, plan)
	must(err)
	job, err := s.ClaimManagement(ctx, project.ID, true, true)
	must(err)
	start := time.Now().UTC().Truncate(time.Second)
	r := ManagementProcessReport{TaskID: task.ID, Revision: 1, Lease: job.Lease, Node: "manager-node", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Session: "manager-thread", Started: start, Output: &ModelOutput{Sequence: 1, Text: "Reviewing.\n"}}}
	conflict := func(report ManagementProcessReport, projectID int64) {
		t.Helper()
		if err := s.ObserveManagementProcess(ctx, projectID, report); !errors.Is(err, ErrConflict) {
			t.Fatal("report accepted", err)
		}
	}
	conflict(r, project.ID+1)
	for _, change := range []func(*ManagementProcessReport){
		func(p *ManagementProcessReport) { p.Lease = strings.Repeat("x", 36) }, func(p *ManagementProcessReport) { p.Revision++ },
		func(p *ManagementProcessReport) { p.Process.Output = &ModelOutput{Sequence: 1, Text: job.Lease + "\n"} },
	} {
		bad := r
		change(&bad)
		conflict(bad, project.ID)
	}
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	for _, change := range []func(*ManagementProcessReport){
		func(p *ManagementProcessReport) { p.Node = "other" }, func(p *ManagementProcessReport) { p.Process.Instance++ },
		func(p *ManagementProcessReport) { p.Process.PID++ }, func(p *ManagementProcessReport) { p.Process.Output = nil },
		func(p *ManagementProcessReport) { p.Process.Output = &ModelOutput{Sequence: 1, Text: "Changed.\n"} },
	} {
		bad := r
		change(&bad)
		conflict(bad, project.ID)
	}
	review := ManagementReview{Level: "D2", Verdict: "approve", Rationale: "Sound", Evidence: "README", SessionID: r.Process.Session}
	if err = s.CompleteManagement(ctx, project.ID, task.ID, 1, job.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal("live accepted", err)
	}
	end := start.Add(time.Second)
	code := 0
	r.Process.Exited = &end
	r.Process.ExitCode = &code
	r.Process.Output = &ModelOutput{Sequence: 2, Text: "Reviewed.\n"}
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	bad := r
	bad.Process.Output = &ModelOutput{Sequence: 3, Text: "Changed after exit.\n"}
	conflict(bad, project.ID)
	wrong := review
	wrong.SessionID = "another-thread"
	if err = s.CompleteManagement(ctx, project.ID, task.ID, 1, job.Lease, wrong); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong session", err)
	}
	must(s.CompleteManagement(ctx, project.ID, task.ID, 1, job.Lease, review))
	must(s.CompleteManagement(ctx, project.ID, task.ID, 1, job.Lease, review))
	conflict(r, project.ID)
	current, err := s.Task(ctx, task.ID)
	must(err)
	historical, err := s.PlanRevision(ctx, task.ID, 1)
	must(err)
	if len(current.Plan.ReviewProcesses) != 1 || len(historical.ReviewProcesses) != 1 || historical.ReviewProcesses[0].Output.Text != "Reviewed.\n" {
		t.Fatal("missing history")
	}
	// An expired attempt remains visible when another worker claims the same revision.
	_, err = s.SavePlan(ctx, task.ID, 1, plan)
	must(err)
	job, err = s.ClaimManagement(ctx, project.ID, true, true)
	must(err)
	r.Revision = 2
	r.Lease = job.Lease
	r.Process.Exited = nil
	r.Process.ExitCode = nil
	r.Process.Output = nil
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	_, err = s.db.ExecContext(ctx, "UPDATE plan_revisions SET review_until=now() WHERE task_id=$1 AND revision=2", task.ID)
	must(err)
	conflict(r, project.ID)
	job, err = s.ClaimManagement(ctx, project.ID, true, true)
	must(err)
	r.Lease = job.Lease
	r.Node = "replacement-node"
	r.Process.PID = 456
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	historical, err = s.PlanRevision(ctx, task.ID, 2)
	must(err)
	if len(historical.ReviewProcesses) != 2 || historical.ReviewProcesses[0].Node == historical.ReviewProcesses[1].Node {
		t.Fatal("lost expired attempt")
	}
}

func TestManagementSessionRecoveryKeepsEveryProcess(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, "management-output")
	must(err)
	task, err := s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{ID: 1, Number: 1, RepositoryID: 9}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	must(err)
	plan := Plan{Objective: "Guide", Milestones: []Milestone{{Name: "Guide", Items: []PlanItem{{Key: "guide", Objective: "Guide", Scope: "README", Deliverables: "README", Acceptance: "Read guide"}}}}}
	_, err = s.SavePlan(ctx, task.ID, 0, plan)
	must(err)
	job, err := s.ClaimManagement(ctx, project.ID, true, true)
	must(err)
	start := time.Now().UTC().Truncate(time.Second)
	r := ManagementProcessReport{TaskID: task.ID, Revision: 1, Lease: job.Lease, Node: "manager-node", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Session: "manager-thread", Started: start, Output: &ModelOutput{Sequence: 1, Text: "Reviewing.\n"}}}

	must(s.ObserveManagementProcess(ctx, project.ID, r))
	next := r
	next.ResumedFrom = 1
	next.Process.Instance = 2
	next.Process.PID = 456
	next.Process.Output = nil
	if err = s.ObserveManagementProcess(ctx, project.ID, next); !errors.Is(err, ErrConflict) {
		t.Fatal("live predecessor resumed", err)
	}
	end := start.Add(time.Second)
	unknown := -1
	r.Process.Exited = &end
	r.Process.ExitCode = &unknown
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	next.Process.Started = end
	bad := next
	bad.Process.Session = "different"
	if err = s.ObserveManagementProcess(ctx, project.ID, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("session changed", err)
	}
	bad = next
	bad.Node = "different"
	if err = s.ObserveManagementProcess(ctx, project.ID, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("node changed", err)
	}
	must(s.ObserveManagementProcess(ctx, project.ID, next))
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	review := ManagementReview{Level: "D2", Verdict: "approve", Rationale: "Sound", Evidence: "README", SessionID: r.Process.Session}
	if err = s.CompleteManagement(ctx, project.ID, task.ID, 1, job.Lease, review); !errors.Is(err, ErrConflict) {
		t.Fatal("latest active process ignored", err)
	}
	end = end.Add(time.Second)
	zero := 0
	next.Process.Exited = &end
	next.Process.ExitCode = &zero
	must(s.ObserveManagementProcess(ctx, project.ID, next))
	must(s.CompleteManagement(ctx, project.ID, task.ID, 1, job.Lease, review))
	history, err := s.PlanRevision(ctx, task.ID, 1)
	must(err)
	if len(history.ReviewProcesses) != 2 || history.ReviewProcesses[0].PID != 123 || *history.ReviewProcesses[0].ExitCode != -1 || history.ReviewProcesses[1].PID != 456 {
		t.Fatal("lost process history", history.ReviewProcesses)
	}
}

func TestManagementMissingSessionHandoffKeepsFailedAttempt(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	project, err := s.CreateProject(ctx, "management-output")
	must(err)
	task, err := s.ImportIssue(ctx, project.ID, IssueSpec{Issue: Issue{ID: 1, Number: 1, RepositoryID: 9}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"})
	must(err)
	plan := Plan{Objective: "Guide", Milestones: []Milestone{{Name: "Guide", Items: []PlanItem{{Key: "guide", Objective: "Guide", Scope: "README", Deliverables: "README", Acceptance: "Read guide"}}}}}
	_, err = s.SavePlan(ctx, task.ID, 0, plan)
	must(err)
	job, err := s.ClaimManagement(ctx, project.ID, true, true)
	must(err)
	start := time.Now().UTC().Truncate(time.Second)
	r := ManagementProcessReport{TaskID: task.ID, Revision: 1, Lease: job.Lease, Node: "manager-node", Process: HarnessProcess{Instance: 1, PID: 123, Harness: "codex", Version: "1.2.3", Session: "manager-thread", Started: start, Output: &ModelOutput{Sequence: 1, Text: "Reviewing.\n"}}}

	must(s.ObserveManagementProcess(ctx, project.ID, r))

	end := start.Add(time.Second)
	unknown := -1
	r.Process.Exited = &end
	r.Process.ExitCode = &unknown
	must(s.ObserveManagementProcess(ctx, project.ID, r))
	missing := r
	missing.ResumedFrom = 1
	missing.Process.Instance = 2
	missing.Process.PID = 456
	missing.Process.Session = ""
	missing.Process.Output = nil
	missing.Process.Started = end
	missing.Process.Exited = nil
	missing.Process.ExitCode = nil
	must(s.ObserveManagementProcess(ctx, project.ID, missing))
	ended := end.Add(time.Second)
	one := 1
	missing.Process.Exited = &ended
	missing.Process.ExitCode = &one
	missing.MissingSession = true
	must(s.ObserveManagementProcess(ctx, project.ID, missing))
	next := missing
	next.ResumedFrom = 2
	next.MissingSession = false
	next.Process.Instance = 3
	next.Process.PID = 789
	next.Process.Session = "replacement-thread"
	next.Process.HandoffFrom = r.Process.Session
	next.Process.Started = ended
	next.Process.Exited = nil
	next.Process.ExitCode = nil
	bad := next
	bad.Process.HandoffFrom = "wrong"
	if err = s.ObserveManagementProcess(ctx, project.ID, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("wrong handoff accepted", err)
	}
	must(s.ObserveManagementProcess(ctx, project.ID, next))
	bad = missing
	bad.MissingSession = false
	if err = s.ObserveManagementProcess(ctx, project.ID, bad); !errors.Is(err, ErrConflict) {
		t.Fatal("missing-session evidence removed", err)
	}
	zero := 0
	finished := ended.Add(time.Second)
	next.Process.Exited = &finished
	next.Process.ExitCode = &zero
	must(s.ObserveManagementProcess(ctx, project.ID, next))
	review := ManagementReview{Level: "D2", Verdict: "approve", Rationale: "Sound", Evidence: "README", SessionID: next.Process.Session}
	must(s.CompleteManagement(ctx, project.ID, task.ID, 1, job.Lease, review))
	history, err := s.PlanRevision(ctx, task.ID, 1)
	must(err)
	if len(history.ReviewProcesses) != 3 || !history.ReviewProcesses[1].MissingSession || history.ReviewProcesses[2].ResumedFrom != 2 || history.ReviewProcesses[2].HandoffFrom != r.Process.Session {
		t.Fatal("lost handoff history", history.ReviewProcesses)
	}
}
