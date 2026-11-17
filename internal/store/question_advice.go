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
	RuleContext         json.RawMessage `json:"rule_context,omitempty"`
	ReferenceDecisionID int64           `json:"reference_decision_id,omitempty"`
	ID                  int64           `json:"id"`
	ExecutionID         int64           `json:"execution_id"`
	Commit              string          `json:"commit"`
	Question            string          `json:"question"`
	Sources             json.RawMessage `json:"sources"`
}

type QuestionAdvice struct {
	Context    *json.RawMessage `json:"context,omitempty"`
	Profile    *AgentProfile    `json:"profile,omitempty"`
	At         *time.Time       `json:"at,omitempty"`
	Level      string           `json:"level"`
	Rationale  string           `json:"rationale"`
	Suggestion string           `json:"suggestion"`
	Source     string           `json:"source"`
	Quote      string           `json:"quote"`
	Session    string           `json:"session_id"`
}

const liveAdviceQuestion = `t.status='WAITING_INPUT' AND q.answer IS NULL AND q.withdrawn_at IS NULL
	AND (q.checkpoint_sequence IS NULL OR (t.waiting_execution_id=e.id AND e.lease_until>clock_timestamp()
	AND e.owner_cancel_requested_at IS NULL AND t.source_closure_id IS NULL))`

func (s *Store) ClaimQuestionAdvice(ctx context.Context, project int64, lookups, supportsDeadline, supportsContext bool) (*ManagementJob, error) {
	var input QuestionAdviceJob
	var rules []byte
	job := ManagementJob{ProjectID: project, Question: &input, MinimumLevel: "D2", CaptureProfile: true}
	tx, pool, err := s.beginManagementClaim(ctx, project, "question_advice", supportsDeadline)
	if tx == nil {
		return nil, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT q.id,q.task_id,q.execution_id,q.commit_sha,q.question,q.evidence,q.reference_decision_id,
			CASE WHEN $5 THEN question_rule_context(q.id) END AS rule_context,
			CASE WHEN d.id IS NOT NULL THEN jsonb_build_object('id','decision:'||d.id,'text',d.statement,'validity',d.validity,
				'valid_until',d.valid_until,'source',d.source,'evidence_refs',d.evidence_refs) END AS reference,rr.id AS requirement,rr.issue,
			CASE WHEN impl.id IS NULL THEN e.decision_context ELSE impl.decision_context END AS decision_context,
			CASE WHEN p.minimum_level='D3' OR p.review->>'effective_level'='D3' OR p.review->>'level'='D3' THEN 'D3' WHEN d.id IS NOT NULL THEN 'D1' ELSE 'D2' END AS minimum
		FROM decision_questions q JOIN work_items t ON t.id=q.task_id JOIN executions e ON e.id=q.execution_id
		LEFT JOIN project_decision_index d ON d.project_id=t.project_id AND d.id=q.reference_decision_id AND d.status='CONFIRMED'
		LEFT JOIN requirement_revisions rr ON rr.id=e.requirement_id
		LEFT JOIN executions impl ON impl.id=CASE WHEN q.checkpoint_sequence IS NULL THEN candidate_implementation_execution(e.id) END
		LEFT JOIN plan_revisions p ON p.task_id=COALESCE(t.goal_id,t.id) AND p.revision=q.plan_revision
		LEFT JOIN question_advice_claims c ON c.question_id=q.id
		WHERE t.project_id=$1 AND (q.reference_decision_id IS NULL OR $2) AND q.management_advice IS NULL AND (c.lease_until IS NULL OR c.lease_until<=clock_timestamp())
		AND `+liveAdviceQuestion+` ORDER BY q.id LIMIT 1 FOR UPDATE OF q SKIP LOCKED
	), claimed AS (
		INSERT INTO question_advice_claims(question_id,lease,lease_until,model_budget,minimum_level,sources,rule_context)
		SELECT id,gen_random_uuid(),clock_timestamp()+interval '6 minutes',$3,minimum,
			jsonb_build_array(jsonb_build_object('id','question','text',evidence)) ||
			CASE WHEN requirement IS NULL THEN '[]'::jsonb ELSE jsonb_build_array(
				jsonb_build_object('id','requirement:'||requirement||':title','text',issue->>'title'),
				jsonb_build_object('id','requirement:'||requirement||':body','text',issue->>'body')) END ||
			COALESCE((SELECT jsonb_agg(jsonb_build_object('id','decision:'||(d->>'id'),'text',d->>'statement'))
				FROM jsonb_array_elements(decision_context) d WHERE d->>'id' IS DISTINCT FROM reference_decision_id::text),'[]'::jsonb) ||
			CASE WHEN reference IS NULL THEN '[]'::jsonb ELSE jsonb_build_array(reference) END || question_context_sources(rule_context),rule_context
		FROM candidate ON CONFLICT(question_id) DO UPDATE SET lease=EXCLUDED.lease,lease_until=EXCLUDED.lease_until,model_budget=EXCLUDED.model_budget,sources=EXCLUDED.sources,minimum_level=EXCLUDED.minimum_level,rule_context=EXCLUDED.rule_context
		RETURNING question_id,lease,sources,minimum_level,lease_until,rule_context
	) SELECT COALESCE(c.reference_decision_id,0),c.id,c.task_id,c.execution_id,c.commit_sha,c.question,r.lease,r.sources,r.minimum_level,CASE WHEN $4 THEN r.lease_until END,r.rule_context FROM candidate c JOIN claimed r ON r.question_id=c.id`, project, lookups, pool, supportsDeadline, supportsContext).Scan(
		&input.ReferenceDecisionID, &input.ID, &job.TaskID, &input.ExecutionID, &input.Commit, &input.Question, &job.Lease, &input.Sources, &job.MinimumLevel, &job.LeaseUntil, &rules)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	input.RuleContext = rules
	return &job, tx.Commit()
}

func (s *Store) CompleteQuestionAdvice(ctx context.Context, project, id int64, lease string, a QuestionAdvice) error {
	if a.Context != nil || !validStructuredProfile(a.Profile) || a.At != nil || (a.Level != "D1" && a.Level != "D2" && a.Level != "D3") || !decisionText(a.Rationale) || !decisionText(a.Source) || !decisionText(a.Quote) ||
		(a.Suggestion != "" && (!decisionText(a.Suggestion) || a.Level != "D2")) || a.Session == "" || len(a.Session) > 128 ||
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
	if a.Level == "D1" {
		// Decision replacement and Owner answers lock these rows before changing state.
		var locked int64
		err = tx.QueryRowContext(ctx, `SELECT p.id FROM projects p JOIN work_items t ON t.project_id=p.id
			JOIN decision_questions q ON q.task_id=t.id JOIN question_advice_claims c ON c.question_id=q.id
			JOIN executions e ON e.id=q.execution_id WHERE p.id=$1 AND q.id=$2 AND c.lease::text=$3 FOR UPDATE OF p,e,t`, project, id, lease).Scan(&locked)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
	}
	var same, recorded, closed, current bool
	err = tx.QueryRowContext(ctx, `SELECT COALESCE((q.management_advice-'at'-'context')=$4::jsonb,false),q.management_advice IS NOT NULL,
		q.answer IS NOT NULL OR q.withdrawn_at IS NOT NULL,
		(`+liveAdviceQuestion+`) AND c.lease_until>clock_timestamp() AND (c.minimum_level<>'D3' OR $8='D3')
		AND ($8<>'D1' OR (c.minimum_level='D1' AND q.checkpoint_sequence IS NOT NULL AND
			EXISTS(SELECT 1 FROM project_decision_index d WHERE d.project_id=t.project_id AND d.id=q.reference_decision_id
				AND d.status='CONFIRMED' AND (d.valid_until IS NULL OR d.valid_until>clock_timestamp()) AND d.statement=$7 AND 'decision:'||d.id=$6)))
		AND NOT EXISTS(SELECT 1 FROM execution_checkpoints cp WHERE cp.execution_id=e.id AND cp.session_id=$5)
		AND COALESCE(e.result->>'session_id','')<>$5
		AND NOT EXISTS(SELECT 1 FROM executions impl WHERE q.checkpoint_sequence IS NULL AND impl.id=candidate_implementation_execution(e.id) AND impl.result->>'session_id'=$5)
		AND EXISTS(SELECT 1 FROM jsonb_array_elements(c.sources) src WHERE src->>'id'=$6 AND strpos(src->>'text',$7)>0)
		FROM decision_questions q JOIN work_items t ON t.id=q.task_id JOIN executions e ON e.id=q.execution_id
		JOIN question_advice_claims c ON c.question_id=q.id WHERE t.project_id=$1 AND q.id=$2 AND c.lease::text=$3
		FOR UPDATE OF q`, project, id, lease, string(raw), a.Session, a.Source, a.Quote, a.Level).Scan(&same, &recorded, &closed, &current)
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
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{QuestionID: id, Lease: lease}, a.Session); err != nil {
		return err
	}
	if !current {
		if closed {
			if _, err = tx.ExecContext(ctx, `UPDATE question_advice_claims SET lease_until=clock_timestamp()
				WHERE question_id=$1 AND lease::text=$2 AND lease_until>clock_timestamp()`, id, lease); err != nil {
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
		}
		return ErrConflict
	}
	event := "management_question_reviewed"
	if a.Level == "D1" {
		if _, err = tx.ExecContext(ctx, `UPDATE decision_questions q SET answer=$2,answered_by='management',answered_at=clock_timestamp(),
			reference_snapshot=(SELECT src FROM question_advice_claims c,jsonb_array_elements(c.sources) src
				WHERE c.question_id=q.id AND src->>'id'=$3 AND src->>'text'=$2) WHERE q.id=$1`, id, a.Quote, a.Source); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE work_items t SET status='RUNNING',waiting_execution_id=NULL FROM decision_questions q
			WHERE q.id=$1 AND t.id=q.task_id`, id); err != nil {
			return err
		}
		event = "management_decision_answered"
	}
	if _, err = tx.ExecContext(ctx, `UPDATE decision_questions q SET management_advice=$2::jsonb || jsonb_build_object('at',clock_timestamp())
		|| CASE WHEN c.rule_context IS NULL THEN '{}'::jsonb ELSE jsonb_build_object('context',c.rule_context) END
		FROM question_advice_claims c WHERE q.id=$1 AND c.question_id=q.id`, id, string(raw)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type,question_id)
		SELECT task_id,execution_id,$2,id FROM decision_questions WHERE id=$1`, id, event); err != nil {
		return err
	}
	return tx.Commit()
}
