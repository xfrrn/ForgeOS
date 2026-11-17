package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestReworkCarriesAnsweredQuestionsAcrossRejections(t *testing.T) {
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
	base := strings.Repeat("a", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 2, ID: 3, Number: 1}, BaseCommit: base, Validation: "docs"})
	if err != nil {
		t.Fatal(err)
	}
	claim := func(supports bool) *Job {
		t.Helper()
		j, err := s.ClaimForProjects(ctx, "dev", root, []int64{p.ID}, WorkerCapabilities{SupportsCodex: true, SupportsRework: true, SupportsReworkAnswers: supports})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	validate := func(j *Job, sha, session string) {
		t.Helper()
		err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{BaseCommit: base, Commit: sha, SessionID: session, Validation: &ValidationResult{Commit: sha, Profile: "docs"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	first := claim(false)
	if first == nil || len(first.OwnerClarifications) != 0 {
		t.Fatal(first)
	}
	b := strings.Repeat("b", 40)
	validate(first, b, "original")
	task, err = s.AskAtAcceptance(ctx, task.ID, QuestionRequest{ExecutionID: first.ExecutionID, Commit: b, Key: "title", Question: "Which title?", Evidence: "The issue leaves the title open."})
	if err != nil {
		t.Fatal(err)
	}
	id := task.Questions[0].ID
	if _, err = s.AnswerQuestion(ctx, task.ID, id, "Harbor"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Review(ctx, task.ID, first.ExecutionID, b, "reject", "Add the index link.", false, ""); err != nil {
		t.Fatal(err)
	}
	if legacy := claim(false); legacy != nil {
		t.Fatal("old adapter received rework requiring Owner evidence", legacy)
	}
	j := claim(true)
	if j == nil || j.Rework.Note != "Add the index link." {
		t.Fatal(j)
	}
	var answers []struct {
		ID          int64
		Answer      string
		ExecutionID int64 `json:"execution_id"`
	}
	if err = json.Unmarshal(j.OwnerClarifications, &answers); err != nil || len(answers) != 1 || answers[0].Answer != "Harbor" || answers[0].ID != id || answers[0].ExecutionID != first.ExecutionID {
		t.Fatal(answers, err)
	}
	c := strings.Repeat("c", 40)
	validate(j, c, "second")
	var same bool
	if err = s.db.QueryRowContext(ctx, `SELECT review_rules_for_execution($1)->'owner_clarifications'=$2::jsonb`, j.ExecutionID, string(j.OwnerClarifications)).Scan(&same); err != nil || !same {
		t.Fatal("review lost the frozen input", err)
	}
	if _, err = s.Review(ctx, task.ID, j.ExecutionID, c, "reject", "Fix a typo.", false, ""); err != nil {
		t.Fatal(err)
	}
	third := claim(true)
	if third == nil || string(third.OwnerClarifications) != string(j.OwnerClarifications) {
		t.Fatal("lost or duplicated an earlier decision", third)
	}
	var inherited int
	if err = s.db.QueryRowContext(ctx, "SELECT count(*) FROM rework_question_ids($1,99999)", task.ID).Scan(&inherited); err != nil || inherited != 0 {
		t.Fatal("leaked into another requirement", inherited, err)
	}
}
