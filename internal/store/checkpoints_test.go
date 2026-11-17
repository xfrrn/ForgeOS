package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCheckpointHandoffRequiresTheResumedSourceAndFinalHistory(t *testing.T) {
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
	p, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	base := strings.Repeat("a", 40)
	_, err = s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, ID: 2, Number: 3}, BaseCommit: base, Validation: "docs"})
	must(err)
	j, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), nil, WorkerCapabilities{SupportsCodex: true, SupportsCheckpoints: true})
	must(err)
	if j == nil {
		t.Fatal("no job")
	}
	checkpoint := func(seq int, session string, history bool, links ...SessionHandoff) error {
		_, err := s.WorkerCheckpoint(ctx, j.ExecutionID, CheckpointRequest{Lease: j.LeaseToken, Sequence: &seq, History: history, Session: session, Handoffs: links}, true)
		return err
	}
	must(checkpoint(0, "", false))
	must(checkpoint(0, "first", false))
	must(checkpoint(0, "", true))
	conflict(checkpoint(1, "", true))
	must(checkpoint(1, "", false))
	_, err = s.db.ExecContext(ctx, `UPDATE execution_checkpoints SET next_instructions='[{"key":"one","body":"Keep the heading"}]' WHERE execution_id=$1 AND sequence=0`, j.ExecutionID)
	must(err)
	link := SessionHandoff{From: "first", Session: "second", SourceInstance: 3, Instance: 5}
	conflict(checkpoint(1, "second", false, link))
	_, err = s.db.ExecContext(ctx, `INSERT INTO model_pauses(execution_id,resource_step,instance,session_id,parked_at,resumed_at,index_tree,worktree_tree)
		VALUES($1,2,3,'first',now(),now(),$2,$2),($1,3,5,'second',now(),now(),$2,$2)`, j.ExecutionID, base)
	must(err)
	conflict(checkpoint(1, "second", false))
	bad := link
	bad.SourceInstance = 4
	conflict(checkpoint(1, "second", false, bad))
	next := SessionHandoff{From: "second", Session: "third", SourceInstance: 5, Instance: 7}
	conflict(checkpoint(1, "third", false, next))
	must(checkpoint(1, "third", false, link, next))
	must(checkpoint(1, "third", false, link, next))
	conflict(checkpoint(1, "second", false, link))
	var earlier, later, reversed bool
	must(s.db.QueryRowContext(ctx, `SELECT checkpoint_session_continues($1,0,'first','third'),checkpoint_session_continues($1,1,'first','third'),checkpoint_session_continues($1,1,'third','first')`, j.ExecutionID).Scan(&earlier, &later, &reversed))
	if earlier || !later || reversed {
		t.Fatal("handoff chain escaped its checkpoint or direction")
	}
	conflict(s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{BaseCommit: base, ExitCode: 1, Error: "later failure", SessionID: "third"}))
}

func TestOwnerQueueBindsInstructionsToTheNextTurn(t *testing.T) {
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
			t.Fatalf("want conflict, got %v", err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, "queue-next")
	must(err)
	base, sha := strings.Repeat("a", 40), strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, ID: 4, Number: 3}, BaseCommit: base, Validation: "docs"})
	must(err)
	caps := WorkerCapabilities{SupportsCodex: true, SupportsCheckpoints: true, SupportsResources: true}
	must(s.RecordHeartbeat(ctx, WorkerHeartbeat{Worker: "queued", OS: "linux", Arch: "amd64", CPUs: 1, Slots: 1, ModelSlots: 1, CISlots: 1, Capabilities: caps}))
	j, err := s.ClaimForProjects(ctx, "queued", t.TempDir(), nil, caps)
	must(err)
	if j == nil {
		t.Fatal("missing execution")
	}
	cp := func(seq int, session string, supports bool) (CheckpointPage, error) {
		return s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, seq, 0, session, supports)
	}
	add := func(key, body string) error {
		return s.QueueInstruction(ctx, j.ExecutionID, 0, OwnerInstruction{Key: key, Body: body})
	}
	_, err = cp(0, "", true)
	must(err)
	conflict(add("one", "early"))
	ok, err := s.ReserveExecutionResource(ctx, j.ExecutionID, ResourceRequest{Lease: j.LeaseToken, Step: 1, Kind: "model"})
	must(err)
	if !ok {
		t.Fatal("model slot unavailable")
	}
	for i := 0; i < 8; i++ {
		must(add(fmt.Sprint(i), "use subtitle "+fmt.Sprint(i)))
	}
	conflict(add("extra", "queue full"))
	must(add("0", "use subtitle 0"))
	conflict(add("0", "changed"))
	_, err = cp(0, "session", true)
	must(err)
	_, err = cp(1, "", false)
	conflict(err)
	page, err := cp(1, "", true)
	must(err)
	commands, err := s.InstructionMessages(ctx, task.ID, j.ExecutionID, 1)
	must(err)
	if len(commands) != 8 || !strings.Contains(string(commands[7]), `"body":"use subtitle 7"`) {
		t.Fatal("missing ordered Owner command", commands)
	}
	if _, err := s.InstructionMessages(ctx, task.ID+1, j.ExecutionID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("command visible under different task", err)
	}
	var items []OwnerInstruction
	must(json.Unmarshal(page.OwnerInstructions, &items))
	if len(items) != 8 || items[7].Key != "7" || len(page.Comments) != 0 {
		t.Fatal(page)
	}
	repeat, err := cp(1, "", true)
	must(err)
	if string(repeat.OwnerInstructions) != string(page.OwnerInstructions) {
		t.Fatal("checkpoint changed on replay")
	}
	conflict(s.RecordCICandidate(ctx, j.ExecutionID, j.LeaseToken, sha, "session"))
	result := Result{Commit: sha, BaseCommit: base, SessionID: "session", Validation: &ValidationResult{Commit: sha, Profile: "docs"}}
	conflict(s.Complete(ctx, j.ExecutionID, j.LeaseToken, result))
	_, err = cp(1, "different-session", true)
	conflict(err)
	_, err = cp(1, "session", true)
	must(err)
	page, err = cp(2, "", true)
	must(err)
	if len(page.OwnerInstructions) != 0 {
		t.Fatal("stale instructions")
	}
	must(s.RecordCICandidate(ctx, j.ExecutionID, j.LeaseToken, sha, "session"))
	must(s.Complete(ctx, j.ExecutionID, j.LeaseToken, result))
	must(add("0", "use subtitle 0"))
	conflict(add("late", "after completion"))
	rows, err := s.Checkpoints(ctx, task.ID, j.ExecutionID)
	must(err)
	if len(rows) != 3 || string(rows[0].NextInstructions) != string(repeat.OwnerInstructions) || rows[1].SessionID != "session" {
		t.Fatal(rows)
	}
	var basis struct {
		Instructions []struct {
			Sequence int             `json:"sequence"`
			Session  string          `json:"session_id"`
			Items    json.RawMessage `json:"instructions"`
		} `json:"owner_instructions"`
	}
	var raw []byte
	must(s.db.QueryRowContext(ctx, `SELECT review_rules_for_execution($1)`, j.ExecutionID).Scan(&raw))
	must(json.Unmarshal(raw, &basis))
	if len(basis.Instructions) != 1 || basis.Instructions[0].Sequence != 1 || basis.Instructions[0].Session != "session" ||
		string(basis.Instructions[0].Items) != string(rows[0].NextInstructions) {
		t.Fatalf("candidate lost the ordered, confirmed instructions: %s", raw)
	}
}

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
		page, err := s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, seq, after, session, false)
		if err != nil {
			t.Fatal(err)
		}
		return page
	}
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, "wrong", 0, 0, "", false); !errors.Is(err, ErrConflict) {
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
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 1, 0, "", false); !errors.Is(err, ErrConflict) {
		t.Fatal("advanced without turn", err)
	}
	check(0, 0, "native-1")
	check(0, 0, "native-1")
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 0, 0, "other", false); !errors.Is(err, ErrConflict) {
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
	if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 2, 0, "", false); !errors.Is(err, ErrConflict) {
		t.Fatal("closed execution accepted new checkpoint", err)
	}
	out, err := s.Checkpoints(ctx, task.ID, j.ExecutionID)
	if err != nil || len(out) != 3 || out[1].SessionID != "native-2" {
		t.Fatal(out, err)
	}
}
