package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPendingQuestionsCursor(t *testing.T) {
	ctx := context.Background()
	db := isolatedDatabase(t)
	s := New(db)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, "decisions")
	if err != nil {
		t.Fatal(err)
	}
	add := func(first, last int) {
		t.Helper()
		_, err := db.ExecContext(ctx, `WITH tasks AS (
			INSERT INTO work_items(project_id,status,kind,github_issue_id,spec)
			SELECT $1,'WAITING_INPUT','codex_issue',n,jsonb_build_object('issue',jsonb_build_object('title','Decision '||n))
			FROM generate_series($2::int,$3::int) n RETURNING id
		), runs AS (
			INSERT INTO executions(task_id,worker,workspace,lease_hash,lease_until) SELECT id,'runner','/work','',now() FROM tasks RETURNING id,task_id
		) INSERT INTO decision_questions(task_id,execution_id,plan_revision,commit_sha,request_key,question,evidence,requester)
			SELECT task_id,id,0,repeat('a',40),'wording','Which wording?','Candidate diff','owner' FROM runs`, project.ID, first, last)
		if err != nil {
			t.Fatal(err)
		}
	}
	type page struct {
		Items []struct{ ID int64 }
		Next  int64 `json:"next_before"`
	}
	read := func(before int64) page {
		t.Helper()
		raw, err := s.PendingQuestions(ctx, 0, before)
		if err != nil {
			t.Fatal(err)
		}
		var p page
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	add(1, 22)
	first := read(0)
	if len(first.Items) != 20 || first.Next != first.Items[19].ID {
		t.Fatal(first)
	}
	add(23, 23)
	if _, err := db.Exec("UPDATE decision_questions SET answer='Recorded' WHERE id=(SELECT min(id) FROM decision_questions)"); err != nil {
		t.Fatal(err)
	}
	last := read(first.Next)
	if len(last.Items) != 1 || last.Items[0].ID >= first.Next || last.Next != 0 || read(0).Items[0].ID <= first.Items[0].ID {
		t.Fatal(last)
	}
}
