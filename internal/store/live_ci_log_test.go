package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLiveCIOutputBindsCandidateLeaseAndCheck(t *testing.T) {
	for _, profile := range []string{"V1", "go-test"} {
		t.Run(profile, func(t *testing.T) { testLiveCIOutputBindsCandidateLeaseAndCheck(t, profile) })
	}
}

func testLiveCIOutputBindsCandidateLeaseAndCheck(t *testing.T, profile string) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	root := t.TempDir()
	p, err := s.CreateProject(ctx, root)
	must(err)
	base := strings.Repeat("a", 40)
	sha := strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, ID: 1, Number: 1}, BaseCommit: base, Validation: profile})
	must(err)
	j, err := s.ClaimForProjects(ctx, "node", root, []int64{p.ID}, WorkerCapabilities{SupportsCodex: true, SupportsValidation: true, SupportsDeclaredChecks: true, SupportsRiskPolicy: true})
	must(err)
	if j == nil || !j.ReportLiveCILog {
		t.Fatal("missing CI log capability")
	}
	l := LiveCILog{Run: "ci-run-12", Check: "build", Commit: sha, Bytes: 6, Text: "hello\n"}
	post := func(log LiveCILog, lease string, ok bool) {
		t.Helper()
		err := s.ObserveCILog(ctx, j.ExecutionID, CILogReport{Log: log, Lease: lease})
		if ok && err != nil || !ok && !errors.Is(err, ErrConflict) {
			t.Fatal(ok, err)
		}
	}
	post(l, j.LeaseToken, false)
	must(s.RecordCICandidate(ctx, j.ExecutionID, j.LeaseToken, sha, "session"))
	post(l, strings.Repeat("e", 64), false)
	post(l, j.LeaseToken, true)
	detail, err := s.Task(ctx, task.ID)
	must(err)
	first := detail.Execution.LiveCILog
	if first == nil || first.Text != l.Text || first.ObservedAt == nil {
		t.Fatal(first)
	}
	post(l, j.LeaseToken, true)
	detail, err = s.Task(ctx, task.ID)
	must(err)
	if !detail.Execution.LiveCILog.ObservedAt.Equal(*first.ObservedAt) {
		t.Fatal("duplicate changed observation time")
	}
	bad := l
	bad.Text = "other\n"
	post(bad, j.LeaseToken, false)
	bad = l
	bad.Bytes--
	post(bad, j.LeaseToken, false)
	bad = l
	bad.Run = "ci-run-13"
	post(bad, j.LeaseToken, false)
	bad = l
	bad.Commit = base
	post(bad, j.LeaseToken, false)
	bad = l
	bad.Index = 1
	bad.Text = "password=example\n"
	post(bad, j.LeaseToken, false)
	next := l
	next.Index = 1
	next.Check = "unit"
	next.Text = ""
	next.Bytes = 0
	post(next, j.LeaseToken, true)
	post(l, j.LeaseToken, false)
	_, err = s.db.ExecContext(ctx, "UPDATE executions SET lease_until=now()-interval '1 second' WHERE id=$1", j.ExecutionID)
	must(err)
	post(next, j.LeaseToken, false)
	detail, err = s.Task(ctx, task.ID)
	must(err)
	if detail.Execution.LiveCILog != nil {
		t.Fatal("expired lease still shows live log")
	}
	_, err = s.db.ExecContext(ctx, "UPDATE executions SET abandoned_at=now() WHERE id=$1", j.ExecutionID)
	must(err)
	var clear bool
	must(s.db.QueryRowContext(ctx, "SELECT live_ci_log IS NULL FROM executions WHERE id=$1", j.ExecutionID).Scan(&clear))
	if !clear {
		t.Fatal("terminal snapshot retained")
	}
}
