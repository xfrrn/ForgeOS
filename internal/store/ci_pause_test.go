package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCIPausePinsResumedExecution(t *testing.T) {
	t.Run("reported", func(t *testing.T) { testCIPausePinsResumedExecution(t, false) })
	t.Run("lost after checkpoint", func(t *testing.T) { testCIPausePinsResumedExecution(t, true) })
}

func testCIPausePinsResumedExecution(t *testing.T, lost bool) {
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
	_, err = s.PublishGlobalRules(ctx, 0, "Original CI instructions")
	if err != nil {
		t.Fatal(err)
	}
	caps := WorkerCapabilities{SupportsCodex: true, SupportsValidation: true, SupportsGlobalRules: true}
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
	ok(s.RecordCICandidate(ctx, first.ExecutionID, first.LeaseToken, sha, "s1"))
	reject(s.RecordCICandidate(ctx, first.ExecutionID, first.LeaseToken, sha, "s2"))
	reject(s.RecordCICandidate(ctx, first.ExecutionID, first.LeaseToken, other, "s1"))
	ok(s.ControlCI(ctx, first.ExecutionID, false))
	reject(s.ControlCI(ctx, first.ExecutionID, true))
	reject(s.Complete(ctx, first.ExecutionID, first.LeaseToken, Result{Commit: other, ExitCode: 1, Error: "stopped"}))
	if lost {
		_, err = s.db.ExecContext(ctx, "UPDATE executions SET lease_until=now() WHERE id=$1", first.ExecutionID)
		ok(err)
		ok(s.Abandon(ctx, first.ExecutionID))
	} else {
		ok(s.Complete(ctx, first.ExecutionID, first.LeaseToken, Result{Commit: sha, SessionID: "s1", ExitCode: 1, Error: "stopped"}))
	}
	_, err = s.PublishGlobalRules(ctx, 1, "Future CI instructions")
	ok(err)
	for range 2 {
		ok(s.ControlCI(ctx, first.ExecutionID, true))
	}
	second := claim()
	if second.Spec.VerifyCommit != sha || second.ExecutionID == first.ExecutionID || second.Workspace == first.Workspace {
		t.Fatal("resumed wrong candidate", second.Spec)
	}
	reject(s.RecordCICandidate(ctx, second.ExecutionID, second.LeaseToken, other, ""))
	ok(s.RecordCICandidate(ctx, second.ExecutionID, second.LeaseToken, sha, ""))
	var origin int64
	var session, originalRule, originalCommit string
	ok(s.db.QueryRowContext(ctx, `SELECT e.id,e.session_id,r->'global_rules'->>'content',r->>'implementation_commit'
  FROM execution_candidate_identity e, review_rules_for_execution($1) r
  WHERE e.id=candidate_implementation_execution($1)`, second.ExecutionID).Scan(&origin, &session, &originalRule, &originalCommit))
	if origin != first.ExecutionID || session != "s1" || originalRule != "Original CI instructions" || originalCommit != sha {
		t.Fatal("resumed wrong implementation", origin, session, originalRule, originalCommit)
	}
	ok(s.ControlCI(ctx, second.ExecutionID, false))
	ok(s.Complete(ctx, second.ExecutionID, second.LeaseToken, Result{Commit: sha, ExitCode: 1, Error: "stopped"}))
	ok(s.ControlCI(ctx, first.ExecutionID, true)) // delayed duplicate must not resume the second pause
	paused, err := s.Task(ctx, task.ID)
	if err != nil || paused.Status != "PAUSED" {
		t.Fatal(paused.Status, err)
	}
	if !lost {
		// Older nodes recorded the model session only in their final report.
		_, err = s.db.ExecContext(ctx, "UPDATE executions SET ci_candidate_session=NULL WHERE id=$1", first.ExecutionID)
		ok(err)
	}
	for _, event := range paused.Events {
		switch event.Type {
		case "execution_claimed", "ci_pause_requested", "ci_paused", "ci_resumed":
		default:
			continue
		}
		raw, err := s.EventAudit(ctx, p.ID, task.ID, event.ID)
		ok(err)
		var receipt struct {
			Session string `json:"session_id"`
			Payload struct {
				Commit string
				Exit   *int `json:"exit_code"`
			}
		}
		ok(json.Unmarshal(raw, &receipt))
		wantCommit, wantSession := sha, "s1"
		if event.ExecutionID == nil {
			t.Fatalf("missing execution ID for %s", event.Type)
		}
		if *event.ExecutionID == second.ExecutionID || (!lost && event.Type == "ci_pause_requested") {
			wantSession = ""
		}
		if event.Type == "execution_claimed" {
			wantCommit, wantSession = "", ""
		}
		if receipt.Payload.Commit != wantCommit || receipt.Session != wantSession || (receipt.Payload.Exit != nil) != (event.Type == "ci_paused") {
			t.Fatalf("wrong CI evidence for %s: %s", event.Type, raw)
		}
	}
	ok(s.CloseSourceIssue(ctx, p.ID, 8, TrackedIssue{ID: 11, Number: 1}, spec.Issue.URL, "not_planned", now, now))
	reject(s.ControlCI(ctx, second.ExecutionID, true))
	final, err := s.Task(ctx, task.ID)
	if err != nil || final.Status != "CANCELLED" {
		t.Fatal(final.Status, err)
	}
}
