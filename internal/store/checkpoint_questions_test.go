package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCheckpointQuestionsHoldProjectUntilResumeOrSettlement(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, end := range []string{"answer", "cancel", "source-closed", "expired"} {
		t.Run(end, func(t *testing.T) {
			root := t.TempDir()
			p, err := s.CreateProject(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			spec := IssueSpec{Issue: Issue{RepositoryID: p.ID, ID: 20, Number: 1, URL: "https://github.com/acme/sample/issues/1"}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
			task, err := s.ImportIssue(ctx, p.ID, spec)
			if err != nil {
				t.Fatal(err)
			}
			j, err := s.Claim(ctx, "worker", root, true, true, true, true)
			if err != nil || j == nil || j.TaskID != task.ID || !j.RuntimeQuestions {
				t.Fatal(j, err)
			}
			next, err := s.CreateTask(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			r := CheckpointQuestion{Lease: j.LeaseToken, Sequence: &seq, Session: "native", Question: "Which name?", Evidence: "Two names are still proposed."}
			check := func(seq int, session string) error {
				_, err := s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, seq, 0, session)
				return err
			}
			if err := check(0, ""); err != nil {
				t.Fatal(err)
			}
			if _, err = s.AskAtCheckpoint(ctx, j.ExecutionID, r); !errors.Is(err, ErrConflict) {
				t.Fatal("unconfirmed turn", err)
			}
			if err := check(0, r.Session); err != nil {
				t.Fatal(err)
			}
			wrong := r
			wrong.Lease = strings.Repeat("0", 64)
			if _, err = s.AskAtCheckpoint(ctx, j.ExecutionID, wrong); !errors.Is(err, ErrConflict) {
				t.Fatal("wrong lease", err)
			}
			for i := 0; i < 2; i++ {
				if answer, err := s.AskAtCheckpoint(ctx, j.ExecutionID, r); err != nil || answer != "" {
					t.Fatal(answer, err)
				}
			}
			wrong = r
			wrong.Question = "Different?"
			if _, err = s.AskAtCheckpoint(ctx, j.ExecutionID, wrong); !errors.Is(err, ErrConflict) {
				t.Fatal("rewrote pending question", err)
			}
			if err := check(1, ""); !errors.Is(err, ErrConflict) {
				t.Fatal("advanced while waiting", err)
			}
			if err = s.RenewExecutionLease(ctx, j.ExecutionID, j.LeaseToken); err != nil {
				t.Fatal(err)
			}
			task, err = s.Task(ctx, task.ID)
			if err != nil || task.Status != "WAITING_INPUT" || len(task.Questions) != 1 || task.Questions[0].Sequence == nil || task.Questions[0].Requester != "worker" {
				t.Fatal(task, err)
			}
			id := task.Questions[0].ID
			if other, err := s.Claim(ctx, "other", root, true, true, true, true); err != nil || other != nil {
				t.Fatal("released the writer", other, err)
			}
			candidate := strings.Repeat("b", 40)
			result := Result{Commit: candidate, BaseCommit: spec.BaseCommit, SessionID: r.Session, Validation: &ValidationResult{Commit: candidate, Profile: "docs"}}
			switch end {
			case "answer":
				for i := 0; i < 2; i++ {
					if task, err = s.AnswerQuestion(ctx, task.ID, id, "Harbor"); err != nil || task.Status != "RUNNING" {
						t.Fatal(task, err)
					}
					if answer, err := s.AskAtCheckpoint(ctx, j.ExecutionID, r); err != nil || answer != "Harbor" {
						t.Fatal(answer, err)
					}
				}
				if err := check(1, ""); err != nil {
					t.Fatal(err)
				}
				if err := s.RecordCICandidate(ctx, j.ExecutionID, j.LeaseToken, candidate, r.Session); !errors.Is(err, ErrConflict) {
					t.Fatal("CI before resumed turn", err)
				}
				if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); !errors.Is(err, ErrConflict) {
					t.Fatal("success before resumed turn", err)
				}
				if err := check(1, "replacement"); !errors.Is(err, ErrConflict) {
					t.Fatal("switched native session", err)
				}
				if err := check(1, r.Session); err != nil {
					t.Fatal(err)
				}
				if err := check(2, ""); err != nil {
					t.Fatal(err)
				}
				if err := s.RecordCICandidate(ctx, j.ExecutionID, j.LeaseToken, candidate, r.Session); err != nil {
					t.Fatal(err)
				}
				if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, result); err != nil {
					t.Fatal(err)
				}
			case "cancel", "source-closed":
				if end == "cancel" {
					err = s.CancelExecution(ctx, j.ExecutionID)
				} else {
					now := time.Now()
					err = s.CloseSourceIssue(ctx, p.ID, p.ID, TrackedIssue{ID: 20, Number: 1}, spec.Issue.URL, "not_planned", now, now)
				}
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.AnswerQuestion(ctx, task.ID, id, "late"); !errors.Is(err, ErrConflict) {
					t.Fatal("accepted cancelled question", err)
				}
				if other, err := s.Claim(ctx, "other", root, true); err != nil || other != nil {
					t.Fatal("released before stop receipt", other, err)
				}
				if err := s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{ExitCode: 1, Error: "stopped"}); err != nil {
					t.Fatal(err)
				}
			case "expired":
				if _, err = s.db.ExecContext(ctx, "UPDATE executions SET lease_until=now() WHERE id=$1", j.ExecutionID); err != nil {
					t.Fatal(err)
				}
				if _, err = s.AnswerQuestion(ctx, task.ID, id, "late"); !errors.Is(err, ErrConflict) {
					t.Fatal("accepted expired lease", err)
				}
				if err := s.Abandon(ctx, j.ExecutionID); err != nil {
					t.Fatal(err)
				}
			}
			if end != "answer" {
				task, err = s.Task(ctx, task.ID)
				if err != nil || task.Questions[0].WithdrawnAt == nil || task.Questions[0].Answer != "" {
					t.Fatal(task, err)
				}
			}
			other, err := s.Claim(ctx, "other", root, true)
			if err != nil || other == nil || other.TaskID != next.ID {
				t.Fatal(other, err)
			}
			if err := s.Complete(ctx, other.ExecutionID, other.LeaseToken, Result{Commit: spec.BaseCommit}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
