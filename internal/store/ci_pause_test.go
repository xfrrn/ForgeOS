package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCIPausePinsResumedExecution(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	p, err := s.CreateProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	spec := IssueSpec{Issue: Issue{ID: 11, Number: 1, RepositoryID: 8, URL: "https://github.com/acme/project/issues/1", UpdatedAt: now.Format(time.RFC3339)}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
	task, err := s.ImportIssue(ctx, p.ID, spec)
	if err != nil {
		t.Fatal(err)
	}
	caps := WorkerCapabilities{SupportsCodex: true, SupportsValidation: true}
	claim := func() *Job {
		t.Helper()
		j, err := s.ClaimForProjects(ctx, "worker", root, nil, caps)
		if err != nil || j == nil || !j.ReportCICandidate {
			t.Fatal("claim", err)
		}
		return j
	}
	first := claim()
	sha := strings.Repeat("b", 40)
	other := strings.Repeat("f", 40)
	reject := func(err error) {
		t.Helper()
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("expected conflict: %v", err)
		}
	}
	ok := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	reject(s.ControlCI(ctx, first.ExecutionID, false))
	reject(s.RecordCICandidate(ctx, first.ExecutionID, "wrong", sha, "s1"))
	reject(s.RecordCICandidate(ctx, first.ExecutionID, first.LeaseToken, spec.BaseCommit, "s1"))
	ok(s.RecordCICandidate(ctx, first.ExecutionID, first.LeaseToken, sha, "s1"))
	reject(s.RecordCICandidate(ctx, first.ExecutionID, first.LeaseToken, other, "s1"))
	ok(s.ControlCI(ctx, first.ExecutionID, false))
	reject(s.ControlCI(ctx, first.ExecutionID, true))
	reject(s.Complete(ctx, first.ExecutionID, first.LeaseToken, Result{Commit: other, ExitCode: 1, Error: "stopped"}))
	ok(s.Complete(ctx, first.ExecutionID, first.LeaseToken, Result{Commit: sha, ExitCode: 1, Error: "stopped"}))
	for range 2 {
		ok(s.ControlCI(ctx, first.ExecutionID, true))
	}
	second := claim()
	if second.Spec.VerifyCommit != sha || second.ExecutionID == first.ExecutionID || second.Workspace == first.Workspace {
		t.Fatal("resumed wrong candidate", second.Spec)
	}
	reject(s.RecordCICandidate(ctx, second.ExecutionID, second.LeaseToken, other, ""))
	ok(s.RecordCICandidate(ctx, second.ExecutionID, second.LeaseToken, sha, ""))
	ok(s.ControlCI(ctx, second.ExecutionID, false))
	ok(s.Complete(ctx, second.ExecutionID, second.LeaseToken, Result{Commit: sha, ExitCode: 1, Error: "stopped"}))
	ok(s.ControlCI(ctx, first.ExecutionID, true)) // delayed duplicate must not resume the second pause
	paused, err := s.Task(ctx, task.ID)
	if err != nil || paused.Status != "PAUSED" {
		t.Fatal(paused.Status, err)
	}
	ok(s.CloseSourceIssue(ctx, p.ID, 8, TrackedIssue{ID: 11, Number: 1}, spec.Issue.URL, "not_planned", now, now))
	reject(s.ControlCI(ctx, second.ExecutionID, true))
	final, err := s.Task(ctx, task.ID)
	if err != nil || final.Status != "CANCELLED" {
		t.Fatal(final.Status, err)
	}
}
