package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestManagementQuestionAdviceIsScopedAndLeavesOwnerDecisionPending(t *testing.T) {
	ctx := context.Background()
	s := New(isolatedDatabase(t))
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var task, id int64
	err = s.db.QueryRowContext(ctx, `WITH task AS (
		INSERT INTO work_items(project_id,status,kind,github_issue_id,spec) VALUES($1,'WAITING_INPUT','codex_issue',1,'{"issue":{"title":"Review a title"}}') RETURNING id
	),run AS (
		INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until) SELECT id,'node','/work','','now' FROM task RETURNING id,task_id
	) INSERT INTO decision_questions(task_id,execution_id,plan_revision,commit_sha,request_key,question,evidence,requester)
	SELECT task_id,id,0,repeat('a',40),'wording','Which title?','Two proposed titles.','owner' FROM run RETURNING task_id,id`, project.ID).Scan(&task, &id)
	if err != nil {
		t.Fatal(err)
	}
	claim := func(projectID int64) *ManagementJob {
		j, err := s.ClaimQuestionAdvice(ctx, projectID, false)
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	if _, err = s.db.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type,question_id)
		SELECT task_id,execution_id,'decision_requested',id FROM decision_questions WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if claim(project.ID+1) != nil {
		t.Fatal("cross-project claim")
	}
	j := claim(project.ID)
	if j == nil || j.Question.ID != id || j.MinimumLevel != "D2" || claim(project.ID) != nil {
		t.Fatal("invalid advice reservation")
	}
	a := QuestionAdvice{Level: "D2", Rationale: "Owner selects the title.", Source: "question", Quote: "two unknown names", Session: "management-session"}
	finish := func(pid int64, lease string) error { return s.CompleteQuestionAdvice(ctx, pid, id, lease, a) }
	if !errors.Is(finish(project.ID, j.Lease), ErrConflict) {
		t.Fatal("accepted invented evidence")
	}
	a.Quote = "Two proposed titles."
	if !errors.Is(finish(project.ID+1, j.Lease), ErrConflict) {
		t.Fatal("accepted foreign role")
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE question_advice_claims SET lease_until=now() WHERE question_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	retry := claim(project.ID)
	if retry == nil || retry.Lease == j.Lease || !errors.Is(finish(project.ID, j.Lease), ErrConflict) {
		t.Fatal("accepted superseded lease")
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE question_advice_claims SET minimum_level='D3' WHERE question_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(finish(project.ID, retry.Lease), ErrConflict) {
		t.Fatal("lowered D3 floor")
	}
	a.Level = "D3"
	a.Suggestion = "Choose the first."
	if !errors.Is(finish(project.ID, retry.Lease), ErrConflict) {
		t.Fatal("accepted D3 recommendation")
	}
	a.Suggestion = ""
	if err = finish(project.ID, retry.Lease); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Task(ctx, task)
	if err != nil || detail.Status != "WAITING_INPUT" || len(detail.Questions) != 1 || detail.Questions[0].Answer != "" || detail.Questions[0].ManagementAdvice == nil || detail.Questions[0].ManagementAdvice.At == nil {
		t.Fatal(detail.Status, err)
	}
	if _, err = s.AnswerQuestion(ctx, task, id, "Harbor"); err != nil {
		t.Fatal(err)
	}
	if err = finish(project.ID, retry.Lease); err != nil {
		t.Fatal("lost response replay", err)
	}
	a.Rationale = "Changed after the answer."
	if !errors.Is(finish(project.ID, retry.Lease), ErrConflict) {
		t.Fatal("rewrote historical advice")
	}
	var events int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM task_events WHERE question_id=$1 AND type='management_question_reviewed'`, id).Scan(&events); err != nil || events != 1 {
		t.Fatal(events, err)
	}
}

func TestManagementDecisionLookup(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []string{"confirmed", "expired after claim", "foreign", "owner answered"} {
		t.Run(scenario, func(t *testing.T) {
			s := New(isolatedDatabase(t))
			if err := s.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			project, err := s.CreateProject(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			pid := project.ID
			if scenario == "foreign" {
				other, err := s.CreateProject(ctx, t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				pid = other.ID
			}
			proposal := DecisionProposal{RequestKey: "heading", Statement: "Heading: Build receipt", Source: "style guide", Validity: "Build receipts", EvidenceRefs: []string{"guide:headings"}}
			raw, err := s.ProposeDecision(ctx, pid, proposal)
			if err != nil {
				t.Fatal(err)
			}
			var decision struct{ ID int64 }
			if err = json.Unmarshal(raw, &decision); err != nil {
				t.Fatal(err)
			}
			if _, err = s.Decide(ctx, pid, decision.ID, "CONFIRMED"); err != nil {
				t.Fatal(err)
			}
			spec := IssueSpec{Issue: Issue{RepositoryID: project.ID, ID: 20, Number: 1, URL: "https://github.com/acme/sample/issues/1"}, BaseCommit: strings.Repeat("a", 40), Validation: "docs"}
			task, err := s.ImportIssue(ctx, project.ID, spec)
			if err != nil {
				t.Fatal(err)
			}
			j, err := s.Claim(ctx, "node", root, true, true, true, true)
			if err != nil || j == nil || !j.DecisionLookups || j.TaskID != task.ID {
				t.Fatal(j, err)
			}
			seq := 0
			q := CheckpointQuestion{ReferenceDecisionID: decision.ID, Sequence: &seq, Session: "impl", Lease: j.LeaseToken, ResumeInstance: strings.Repeat("1", 64), Trees: &WorktreeTrees{Index: spec.BaseCommit, Worktree: spec.BaseCommit}}
			q.Question, q.Evidence = DecisionLookupText(decision.ID)
			for _, session := range []string{"", q.Session} {
				if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, seq, 0, session); err != nil {
					t.Fatal(err)
				}
			}
			wrong := q
			wrong.Question = "Approve a change?"
			if _, err = s.AskAtCheckpoint(ctx, j.ExecutionID, wrong); !errors.Is(err, ErrConflict) {
				t.Fatal("free-form lookup accepted", err)
			}
			if _, err = s.AskAtCheckpoint(ctx, j.ExecutionID, q); err != nil {
				t.Fatal(err)
			}
			if old, err := s.ClaimQuestionAdvice(ctx, project.ID, false); err != nil || old != nil {
				t.Fatal("legacy reviewer claimed lookup", old, err)
			}
			m, err := s.ClaimQuestionAdvice(ctx, project.ID, true)
			if err != nil || m == nil || m.Question.ReferenceDecisionID != decision.ID {
				t.Fatal(m, err)
			}
			a := QuestionAdvice{Level: "D1", Source: fmt.Sprintf("decision:%d", decision.ID), Quote: proposal.Statement, Rationale: "The guide covers the requested receipt.", Session: "reviewer"}
			finish := func() error { return s.CompleteQuestionAdvice(ctx, project.ID, m.Question.ID, m.Lease, a) }
			if scenario == "foreign" {
				if m.MinimumLevel != "D2" || strings.Contains(string(m.Question.Sources), proposal.Statement) || !errors.Is(finish(), ErrConflict) {
					t.Fatal("foreign decision disclosed")
				}
				return
			}
			if m.MinimumLevel != "D1" {
				t.Fatal(m.MinimumLevel)
			}
			if scenario == "expired after claim" {
				if _, err = s.Decide(ctx, pid, decision.ID, "EXPIRED"); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(finish(), ErrConflict) {
					t.Fatal("expired decision settled lookup")
				}
				return
			}
			if scenario == "owner answered" {
				if _, err = s.AnswerQuestion(ctx, task.ID, m.Question.ID, "New heading"); err != nil {
					t.Fatal(err)
				}
				if !errors.Is(finish(), ErrConflict) {
					t.Fatal("overwrote Owner")
				}
				return
			}
			a.Quote = "Build receipt"
			if !errors.Is(finish(), ErrConflict) {
				t.Fatal("short quote settled lookup")
			}
			a.Quote = proposal.Statement
			for i := 0; i < 2; i++ {
				if err = finish(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = s.AnswerQuestion(ctx, task.ID, m.Question.ID, a.Quote); !errors.Is(err, ErrConflict) {
				t.Fatal("same Management answer attributed to Owner")
			}
			got, err := s.AskAtCheckpoint(ctx, j.ExecutionID, q)
			if err != nil || got != a.Quote {
				t.Fatal(got, err)
			}
			msg, err := s.QuestionMessages(ctx, task.ID, m.Question.ID)
			if err != nil || len(msg) != 2 || msg[1].From.ID != "management" || msg[1].Trust != "agent_generated" {
				t.Fatal(msg, err)
			}
			if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 1, 0, ""); err != nil {
				t.Fatal(err)
			}
			if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 1, 0, q.Session); err != nil {
				t.Fatal(err)
			}
			if _, err = s.ReadCheckpoint(ctx, j.ExecutionID, j.LeaseToken, 2, 0, ""); err != nil {
				t.Fatal(err)
			}
			sha := strings.Repeat("b", 40)
			if err = s.RecordCICandidate(ctx, j.ExecutionID, j.LeaseToken, sha, q.Session); err != nil {
				t.Fatal(err)
			}
			err = s.Complete(ctx, j.ExecutionID, j.LeaseToken, Result{BaseCommit: spec.BaseCommit, Commit: sha, SessionID: q.Session, Validation: &ValidationResult{Profile: "docs", Commit: sha}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Decide(ctx, pid, decision.ID, "EXPIRED"); err != nil {
				t.Fatal(err)
			}
			if err = s.RequestCodeReview(ctx, task.ID, j.ExecutionID, sha); err != nil {
				t.Fatal(err)
			}
			reviewer, err := s.ClaimCodeReview(ctx, pid, true)
			if err != nil || reviewer == nil {
				t.Fatal(reviewer, err)
			}
			var frozen struct {
				References []struct {
					Question int64  `json:"question_id"`
					Session  string `json:"session_id"`
					Decision struct{ ID, Text, Source, Validity string }
				} `json:"management_references"`
				Owner json.RawMessage `json:"owner_clarifications"`
			}
			if err = json.Unmarshal(reviewer.CandidateContext, &frozen); err != nil || len(frozen.References) != 1 || len(frozen.Owner) != 0 {
				t.Fatal(string(reviewer.CandidateContext), err)
			}
			ref := frozen.References[0]
			if ref.Question != m.Question.ID || ref.Session != a.Session || ref.Decision.ID != a.Source || ref.Decision.Text != proposal.Statement || ref.Decision.Validity != proposal.Validity || ref.Decision.Source != proposal.Source {
				t.Fatal(ref)
			}
		})
	}
}
