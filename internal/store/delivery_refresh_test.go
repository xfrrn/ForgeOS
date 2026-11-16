package store

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRefreshKeepsDeliveryAndRequiresNewValidation(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "refresh")
	if err != nil {
		t.Fatal(err)
	}
	base, old, target, head := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40), strings.Repeat("d", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{ID: 7, Number: 1, RepositoryID: 9}, BaseCommit: base, Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	j, err := s.Claim(ctx, "model", root, true)
	if err != nil || j == nil {
		t.Fatal(j, err)
	}
	r := Result{Commit: old, BaseCommit: base, SessionID: "before", Validation: &ValidationResult{Commit: old, Profile: "docs"}}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, r); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Review(ctx, task.ID, j.ExecutionID, old, "approve", "checked", true, "main"); err != nil {
		t.Fatal(err)
	}
	d, err := s.ClaimDelivery(ctx, p.ID)
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	d.State, d.PRNumber, d.PRURL = "WAITING", 4, "https://github.com/a/b/pull/4"
	if err = s.SaveDelivery(ctx, *d); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		queued, err := s.QueueRefreshedCandidate(ctx, task.ID, j.ExecutionID, old, target, head)
		if err != nil || queued.Status != "READY" || len(queued.Refreshes) != 1 || queued.Delivery != nil || queued.Spec.VerifyCommit != head {
			t.Fatal(queued, err)
		}
	}
	if _, err = s.QueueRefreshedCandidate(ctx, task.ID, j.ExecutionID, old, target, strings.Repeat("f", 40)); !errors.Is(err, ErrConflict) {
		t.Fatal("changed replay", err)
	}
	if j, err := s.Claim(ctx, "old", root, true, true, true, true); err != nil || j != nil {
		t.Fatal(j, err)
	}
	verify, err := s.Claim(ctx, "validator", root, false, false, false, false, true)
	if err != nil || verify == nil || verify.SupportsCheckpoints {
		t.Fatal(verify, err)
	}
	r.SessionID, r.BaseCommit = "", target
	if err = s.Complete(ctx, verify.ExecutionID, verify.LeaseToken, r); !errors.Is(err, ErrConflict) {
		t.Fatal("old SHA validated", err)
	}
	r.Commit, r.Validation.Commit = head, head
	if err = s.Complete(ctx, verify.ExecutionID, verify.LeaseToken, r); err != nil {
		t.Fatal(err)
	}
	fresh, err := s.Task(ctx, task.ID)
	if err != nil || fresh.Status != "WAITING_ACCEPTANCE" || len(fresh.Reviews) != 1 || fresh.Refreshes[0].OldDelivery.Commit != old {
		t.Fatal(fresh, err)
	}
	if _, err = s.Review(ctx, task.ID, verify.ExecutionID, head, "reject", "revise this version", false, ""); err != nil {
		t.Fatal(err)
	}
	redo, err := s.Claim(ctx, "model", root, true, true)
	if err != nil || redo == nil || redo.Rework == nil || redo.Rework.Commit != head || redo.Spec.VerifyCommit != "" {
		t.Fatal(redo, err)
	}
}
