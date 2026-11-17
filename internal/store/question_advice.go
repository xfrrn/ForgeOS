package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type QuestionAdviceJob struct {
	ID          int64           `json:"id"`
	ExecutionID int64           `json:"execution_id"`
	Commit      string          `json:"commit"`
	Question    string          `json:"question"`
	Sources     json.RawMessage `json:"sources"`
}

type QuestionAdvice struct {
	At         *time.Time `json:"at,omitempty"`
	Level      string     `json:"level"`
	Rationale  string     `json:"rationale"`
	Suggestion string     `json:"suggestion"`
	Source     string     `json:"source"`
	Quote      string     `json:"quote"`
	Session    string     `json:"session_id"`
}

const liveAdviceQuestion = `t.status='WAITING_INPUT' AND q.answer IS NULL AND q.withdrawn_at IS NULL
	AND (q.checkpoint_sequence IS NULL OR (t.waiting_execution_id=e.id AND e.lease_until>clock_timestamp()
	AND e.owner_cancel_requested_at IS NULL AND t.source_closure_id IS NULL))`

func (s *Store) ClaimQuestionAdvice(ctx context.Context, project int64) (*ManagementJob, error) {
	var input QuestionAdviceJob
	job := ManagementJob{ProjectID: project, Question: &input, MinimumLevel: "D2"}
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
		SELECT q.id,q.task_id,q.execution_id,q.commit_sha,q.question,q.evidence,rr.id AS requirement,rr.issue,
			CASE WHEN impl.id IS NULL THEN e.decision_context ELSE impl.decision_context END AS decision_context,
			CASE WHEN p.minimum_level='D3' OR p.review->>'effective_level'='D3' OR p.review->>'level'='D3' THEN 'D3' ELSE 'D2' END AS minimum
		FROM decision_questions q JOIN work_items t ON t.id=q.task_id JOIN executions e ON e.id=q.execution_id
		LEFT JOIN requirement_revisions rr ON rr.id=e.requirement_id
		LEFT JOIN executions impl ON impl.id=CASE WHEN q.checkpoint_sequence IS NULL THEN candidate_implementation_execution(e.id) END
		LEFT JOIN plan_revisions p ON p.task_id=COALESCE(t.goal_id,t.id) AND p.revision=q.plan_revision
		LEFT JOIN question_advice_claims c ON c.question_id=q.id
		WHERE t.project_id=$1 AND q.management_advice IS NULL AND (c.lease_until IS NULL OR c.lease_until<=clock_timestamp())
		AND `+liveAdviceQuestion+` ORDER BY q.id LIMIT 1 FOR UPDATE OF q SKIP LOCKED
	), claimed AS (
		INSERT INTO question_advice_claims(question_id,lease,lease_until,minimum_level,sources)
		SELECT id,gen_random_uuid(),clock_timestamp()+interval '6 minutes',minimum,
			jsonb_build_array(jsonb_build_object('id','question','text',evidence)) ||
			CASE WHEN requirement IS NULL THEN '[]'::jsonb ELSE jsonb_build_array(
				jsonb_build_object('id','requirement:'||requirement||':title','text',issue->>'title'),
				jsonb_build_object('id','requirement:'||requirement||':body','text',issue->>'body')) END ||
			COALESCE((SELECT jsonb_agg(jsonb_build_object('id','decision:'||(d->>'id'),'text',d->>'statement'))
				FROM jsonb_array_elements(decision_context) d),'[]'::jsonb)
		FROM candidate ON CONFLICT(question_id) DO UPDATE SET lease=EXCLUDED.lease,lease_until=EXCLUDED.lease_until,sources=EXCLUDED.sources,minimum_level=EXCLUDED.minimum_level
		RETURNING question_id,lease,sources,minimum_level
	) SELECT c.id,c.task_id,c.execution_id,c.commit_sha,c.question,r.lease,r.sources,r.minimum_level FROM candidate c JOIN claimed r ON r.question_id=c.id`, project).Scan(
		&input.ID, &job.TaskID, &input.ExecutionID, &input.Commit, &input.Question, &job.Lease, &input.Sources, &job.MinimumLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) CompleteQuestionAdvice(ctx context.Context, project, id int64, lease string, a QuestionAdvice) error {
	if a.At != nil || (a.Level != "D2" && a.Level != "D3") || !decisionText(a.Rationale) || !decisionText(a.Source) || !decisionText(a.Quote) ||
		(a.Suggestion != "" && (!decisionText(a.Suggestion) || a.Level == "D3")) || a.Session == "" || len(a.Session) > 128 ||
		strings.Trim(a.Session, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return ErrConflict
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var same, recorded, current bool
	err = tx.QueryRowContext(ctx, `SELECT COALESCE((q.management_advice-'at')=$4::jsonb,false),q.management_advice IS NOT NULL,
		(`+liveAdviceQuestion+`) AND c.lease_until>clock_timestamp() AND (c.minimum_level<>'D3' OR $8='D3')
		AND NOT EXISTS(SELECT 1 FROM execution_checkpoints cp WHERE cp.execution_id=e.id AND cp.session_id=$5)
		AND COALESCE(e.result->>'session_id','')<>$5
		AND NOT EXISTS(SELECT 1 FROM executions impl WHERE q.checkpoint_sequence IS NULL AND impl.id=candidate_implementation_execution(e.id) AND impl.result->>'session_id'=$5)
		AND EXISTS(SELECT 1 FROM jsonb_array_elements(c.sources) src WHERE src->>'id'=$6 AND strpos(src->>'text',$7)>0)
		FROM decision_questions q JOIN work_items t ON t.id=q.task_id JOIN executions e ON e.id=q.execution_id
		JOIN question_advice_claims c ON c.question_id=q.id WHERE t.project_id=$1 AND q.id=$2 AND c.lease::text=$3
		FOR UPDATE OF q`, project, id, lease, string(raw), a.Session, a.Source, a.Quote, a.Level).Scan(&same, &recorded, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if recorded {
		if same {
			return tx.Commit()
		}
		return ErrConflict
	}
	if !current {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE decision_questions SET management_advice=$2::jsonb || jsonb_build_object('at',clock_timestamp()) WHERE id=$1`, id, string(raw)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type,question_id)
		SELECT task_id,execution_id,'management_question_reviewed',id FROM decision_questions WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}
