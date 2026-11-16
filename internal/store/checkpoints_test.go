package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCheckpointFreezesPagesAndRequiresConsumedTurns(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "comments")
	if err != nil {
		t.Fatal(err)
	}
	issue := Issue{RepositoryID: 31, ID: 2, Number: 1}
	base := strings.Repeat("a", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: issue, Validation: "docs", BaseCommit: base})
	if err != nil {
		t.Fatal(err)
	}
	record := func(id, second int, body string) {
		t.Helper()
		raw := json.RawMessage(fmt.Sprintf(`{"id":%d,"body":"%s","html_url":"source","updated_at":"2026-02-01T00:00:%02dZ"}`, id, body, second))
		if err := s.RecordComments(ctx, task.ID, issue, []json.RawMessage{raw}); err != nil {
			t.Fatal(err)
		}
	}
	for id := 1; id <= 10; id++ {
		record(id, 0, "initial")
	}
	record(1, 1, "edited")
	root := t.TempDir()
	if j, err := s.Claim(ctx, "old", root, true, true, true); err != nil || j != nil {
		t.Fatal(j, err)
	}
	j, err := s.Claim(ctx, "worker", root, true, true, true, true)
	if err != nil || j == nil || !j.SupportsCheckpoints {
		t.Fatal(j, err)
	}
	check := func(seq int, after int64, session string) CheckpointPage {
		t.Helper()
		page, err := s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, seq, after, session)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, "wrong", 0, 0, ""); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	first := check(0, 0, "")
	if !first.More || len(first.Comments) != 8 {
		t.Fatal(first)
	}
	record(11, 2, "during turn")
	second := check(0, first.Comments[7].EventID, "")
	if second.More || len(second.Comments) != 2 || second.Through != first.Through || second.Comments[1].Body != "edited" {
		t.Fatal(second)
	}
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 1, 0, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("advanced without turn", err)
	}
	check(0, 0, "native-1")
	check(0, 0, "native-1")
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 0, 0, "other"); !errors.Is(err, ErrConflict) {
		t.Fatal("overwritten receipt", err)
	}
	next := check(1, 0, "")
	if len(next.Comments) != 1 || next.Comments[0].Body != "during turn" {
		t.Fatal(next)
	}
	candidate := strings.Repeat("b", 40)
	result := Result{Commit: candidate, BaseCommit: base, SessionID: "native-1", Validation: &ValidationResult{Commit: candidate, Profile: "docs"}}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); !errors.Is(err, ErrConflict) {
		t.Fatal("skipped pending comments", err)
	}
	check(1, 0, "native-2")
	result.SessionID = "native-2"
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); !errors.Is(err, ErrConflict) {
		t.Fatal("skipped commit checkpoint", err)
	}
	end := check(2, 0, "")
	if len(end.Comments) != 0 || end.After != end.Through {
		t.Fatal(end)
	}
	if err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 2, 0, ""); !errors.Is(err, ErrConflict) {
		t.Fatal("closed execution accepted new checkpoint", err)
	}
	out, err := s.Checkpoints(ctx, task.ID, j.ExecutionID)
	if err != nil || len(out) != 3 || out[1].SessionID != "native-2" {
		t.Fatal(out, err)
	}
}
