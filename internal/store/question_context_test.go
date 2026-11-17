package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestQuestionContextKeepsExecutionEvidenceAndCheckpointBoundary(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Migrate(ctx))
	p, err := s.CreateProject(ctx, t.TempDir())
	must(err)
	base, sha := strings.Repeat("a", 40), strings.Repeat("b", 40)
	task, err := s.ImportIssue(ctx, p.ID, IssueSpec{Issue: Issue{RepositoryID: 1, ID: 2, Number: 3, Title: "Owner request"}, BaseCommit: base, Validation: "docs"})
	must(err)
	_, err = s.PublishGlobalRules(ctx, 0, "Original fallback")
	must(err)
	caps := WorkerCapabilities{SupportsCodex: true, SupportsGlobalRules: true, SupportsRework: true, SupportsReworkAnswers: true}
	first, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), nil, caps)
	must(err)
	if first == nil {
		t.Fatal("no first execution")
	}
	must(s.Complete(ctx, first.ExecutionID, first.LeaseToken, Result{BaseCommit: base, Commit: sha, SessionID: "first", Validation: &ValidationResult{Profile: "docs", Commit: sha}}))
	task, err = s.AskAtAcceptance(ctx, task.ID, QuestionRequest{ExecutionID: first.ExecutionID, Commit: sha, Key: "title", Question: "Which heading?", Evidence: "Two headings"})
	must(err)
	inherited := task.Questions[0].ID
	_, err = s.AnswerQuestion(ctx, task.ID, inherited, "Owner heading")
	must(err)
	_, err = s.Review(ctx, task.ID, first.ExecutionID, sha, "reject", "Keep the Owner heading", false, "")
	must(err)
	caps.SupportsCheckpoints = true
	job, err := s.ClaimForProjects(ctx, "writer", t.TempDir(), nil, caps)
	must(err)
	if job == nil {
		t.Fatal("no rework")
	}
	check := func(seq int, session string) {
		t.Helper()
		_, err := s.ReadCheckpoint(ctx, job.ExecutionID, job.LeaseToken, seq, 0, session, true)
		must(err)
	}
	ask := func(seq int) int64 {
		t.Helper()
		_, err := s.AskAtCheckpoint(ctx, job.ExecutionID, CheckpointQuestion{Lease: job.LeaseToken, Sequence: &seq, Session: "rework", Question: "Which footer?", Evidence: "Two footers", ResumeInstance: strings.Repeat("c", 64), Trees: &WorktreeTrees{Index: base, Worktree: base}})
		must(err)
		detail, err := s.Task(ctx, task.ID)
		must(err)
		for _, q := range detail.Questions {
			if q.ExecutionID == job.ExecutionID && q.Sequence != nil && *q.Sequence == seq {
				return q.ID
			}
		}
		t.Fatal("missing question")
		return 0
	}
	check(0, "")
	check(0, "rework")
	previous := ask(0)
	_, err = s.AnswerQuestion(ctx, task.ID, previous, "Owner footer")
	must(err)
	check(1, "")
	check(1, "rework")
	id := ask(1)
	// Seed later checkpoints to prove historical questions cannot see future processed commands.
	_, err = s.db.ExecContext(ctx, `UPDATE execution_checkpoints SET next_instructions=$3::jsonb WHERE execution_id=$1 AND sequence=$2`, job.ExecutionID, 0, `[{"key":"heading","body":"Processed Owner heading"}]`)
	must(err)
	_, err = s.db.ExecContext(ctx, `UPDATE execution_checkpoints SET next_instructions=$2::jsonb WHERE execution_id=$1 AND sequence=1`, job.ExecutionID, `[{"key":"later","body":"Future processed command"}]`)
	must(err)
	_, err = s.db.ExecContext(ctx, `INSERT INTO execution_checkpoints(execution_id,sequence,source_task_id,after_event,through_event,session_id,next_instructions) VALUES($1,2,$2,0,0,'rework','[{"key":"pending","body":"Unprocessed command"}]')`, job.ExecutionID, task.ID)
	must(err)
	_, err = s.PublishGlobalRules(ctx, 1, "Later fallback")
	must(err)
	claim := func(supports bool) *ManagementJob {
		t.Helper()
		j, err := s.ClaimQuestionAdvice(ctx, p.ID, true, true, supports)
		must(err)
		if j == nil || j.Question.ID != id {
			t.Fatal("missing claim")
		}
		return j
	}
	expire := func() {
		_, err := s.db.ExecContext(ctx, `UPDATE question_advice_claims SET lease_until=now() WHERE question_id=$1`, id)
		must(err)
	}
	j := claim(true)
	var evidence struct {
		Rules struct {
			Revision int
			Content  string
		} `json:"global_rules"`
		Answers []struct {
			ID     int64
			Answer string
		} `json:"owner_clarifications"`
		Feedback     struct{ Note string } `json:"acceptance_feedback"`
		Instructions []struct {
			Sequence     int
			Instructions []OwnerInstruction
		} `json:"owner_instructions"`
	}
	must(json.Unmarshal(j.Question.RuleContext, &evidence))
	if evidence.Rules.Revision != 1 || evidence.Rules.Content != "Original fallback" || evidence.Feedback.Note != "Keep the Owner heading" ||
		len(evidence.Answers) != 2 || evidence.Answers[0].ID != inherited || evidence.Answers[1].ID != previous ||
		len(evidence.Instructions) != 1 || evidence.Instructions[0].Sequence != 1 {
		t.Fatalf("wrong context: %s", j.Question.RuleContext)
	}
	var sources []struct{ ID, Text string }
	must(json.Unmarshal(j.Question.Sources, &sources))
	got := map[string]string{}
	for _, source := range sources {
		got[source.ID] = source.Text
	}
	for key, want := range map[string]string{"global-rule:1": "Original fallback", fmt.Sprint("acceptance-feedback:", first.ExecutionID): "Keep the Owner heading", fmt.Sprint("owner-answer:", inherited): "Owner heading", fmt.Sprint("owner-answer:", previous): "Owner footer", "owner-command:1:heading": "Processed Owner heading"} {
		if got[key] != want {
			t.Fatalf("source %s = %q, want %q", key, got[key], want)
		}
	}
	for _, excluded := range []string{"Later fallback", "Future processed command", "Unprocessed command"} {
		if strings.Contains(string(j.Question.Sources), excluded) || strings.Contains(string(j.Question.RuleContext), excluded) {
			t.Fatal("disclosed future input", excluded)
		}
	}
	expire()
	old := claim(false)
	if old.Question.RuleContext != nil || strings.Contains(string(old.Question.Sources), "global-rule:") {
		t.Fatal("legacy claim retained unused context")
	}
	expire()
	j = claim(true)
	a := QuestionAdvice{Level: "D2", Rationale: "Use explicit Owner input", Source: "owner-command:1:heading", Quote: "Processed Owner heading", Session: "independent"}
	finish := func() error { return s.CompleteQuestionAdvice(ctx, p.ID, id, j.Lease, a) }
	forged := json.RawMessage(`{}`)
	a.Context = &forged
	if !errors.Is(finish(), ErrConflict) {
		t.Fatal("accepted forged context")
	}
	a.Context = nil
	a.Source, a.Quote = "global-rule:2", "Later fallback"
	if !errors.Is(finish(), ErrConflict) {
		t.Fatal("accepted new global rule")
	}
	a.Source, a.Quote = "owner-command:1:heading", "Processed Owner heading"
	must(finish())
	_, err = s.AnswerQuestion(ctx, task.ID, id, "Selected footer")
	must(err)
	must(finish())
	detail, err := s.Task(ctx, task.ID)
	must(err)
	for _, q := range detail.Questions {
		if q.ID == id {
			if q.ManagementAdvice == nil || q.ManagementAdvice.Context == nil || string(*q.ManagementAdvice.Context) != string(j.Question.RuleContext) {
				t.Fatal("receipt lost frozen context")
			}
		}
	}
}
