package store

import (
	"context"
	"errors"
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
		j, err := s.ClaimQuestionAdvice(ctx, projectID)
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
