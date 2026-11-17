package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type QuestionRequest struct {
	FeedbackID  int64  `json:"feedback_id,omitempty"`
	ExecutionID int64  `json:"execution_id"`
	Commit      string `json:"commit_sha"`
	Key         string `json:"request_key"`
	Question    string `json:"question"`
	Evidence    string `json:"evidence"`
}

type DecisionQuestion struct {
	QuestionRequest
	ID           int64          `json:"id"`
	PlanRevision int            `json:"plan_revision"`
	Sequence     *int           `json:"checkpoint_sequence,omitempty"`
	Trees        *WorktreeTrees `json:"trees,omitempty"`
	WithdrawnAt  *time.Time     `json:"withdrawn_at,omitempty"`
	Requester    string         `json:"requester"`
	At           time.Time      `json:"at"`
	Answer       string         `json:"answer,omitempty"`
	AnsweredBy   string         `json:"answered_by,omitempty"`
	AnsweredAt   *time.Time     `json:"answered_at,omitempty"`
}

func (s *Store) PendingQuestions(ctx context.Context, before int64) (json.RawMessage, error) {
	var page json.RawMessage
	err := s.db.QueryRowContext(ctx, `WITH pending AS (
		SELECT q.id,q.task_id,t.project_id,q.execution_id,q.commit_sha,q.question,q.evidence,q.checkpoint_sequence,
		       t.spec->'issue'->>'repository' AS repository,t.spec->'issue'->>'title' AS title
		FROM decision_questions q JOIN work_items t ON t.id=q.task_id
		WHERE q.withdrawn_at IS NULL AND q.answer IS NULL AND t.status='WAITING_INPUT' AND ($1::bigint=0 OR q.id<$1)
		ORDER BY q.id DESC LIMIT 21
	), page AS (SELECT * FROM pending ORDER BY id DESC LIMIT 20)
	SELECT jsonb_build_object('items',COALESCE(jsonb_agg(to_jsonb(page) ORDER BY id DESC),'[]'),
		'next_before',CASE WHEN (SELECT count(*) FROM pending)>20 THEN min(id) END) FROM page`, before).Scan(&page)
	return page, err
}

func decisionText(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0) && len(s) <= 4096
}

func (r QuestionRequest) Valid() bool {
	return r.ExecutionID > 0 && validCommit(r.Commit) && planItemKey.MatchString(r.Key) && decisionText(r.Question) && decisionText(r.Evidence) &&
		(r.FeedbackID == 0 || (r.FeedbackID > 0 && r.Key == "feedback-"+strconv.FormatInt(r.FeedbackID, 10)))
}

func (s *Store) AskAtAcceptance(ctx context.Context, id int64, r QuestionRequest) (Task, error) {
	if !r.Valid() {
		return Task{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var status string
	var revision int
	err = tx.QueryRowContext(ctx, `SELECT status,COALESCE(plan_revision,0) FROM work_items WHERE id=$1 FOR UPDATE`, id).Scan(&status, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT question=$3 AND evidence=$4 AND commit_sha=$5 FROM decision_questions
		WHERE task_id=$1 AND execution_id=$2 AND request_key=$6 AND feedback_id IS NOT DISTINCT FROM NULLIF($7::bigint,0)`, id, r.ExecutionID, r.Question, r.Evidence, r.Commit, r.Key, r.FeedbackID).Scan(&same)
	if err == nil {
		if !same {
			return Task{}, ErrConflict
		}
		if err := tx.Commit(); err != nil {
			return Task{}, err
		}
		return s.Task(ctx, id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Task{}, err
	}
	if status != "WAITING_ACCEPTANCE" {
		return Task{}, ErrConflict
	}
	var qID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO decision_questions(task_id,execution_id,plan_revision,commit_sha,request_key,question,evidence,requester,feedback_id)
		SELECT $1,id,$3,$4,$5,$6,$7,'owner',NULLIF($8::bigint,0) FROM executions WHERE task_id=$1 AND id=$2
		AND id=(SELECT max(id) FROM executions WHERE task_id=$1) AND result->>'commit'=$4
		AND ($8=0 OR EXISTS(SELECT 1 FROM preview_feedback f JOIN static_previews p ON p.id=f.preview_id
			WHERE f.id=$8 AND p.execution_id=$2 AND f.classification->>'category'='blocked' AND f.classification->>'evidence'=$7))
		RETURNING id`, id, r.ExecutionID, revision, r.Commit, r.Key, r.Question, r.Evidence, r.FeedbackID).Scan(&qID)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrConflict
	}
	if err != nil {
		return Task{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='WAITING_INPUT' WHERE id=$1`, id); err != nil {
		return Task{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type,question_id) VALUES ($1,$2,'decision_requested',$3)`, id, r.ExecutionID, qID); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}

type CheckpointQuestion struct {
	ResumeInstance string         `json:"resume_instance,omitempty"`
	Trees          *WorktreeTrees `json:"trees,omitempty"`
	Lease          string         `json:"lease_token"`
	Sequence       *int           `json:"sequence"`
	Session        string         `json:"session_id"`
	Question       string         `json:"question"`
	Evidence       string         `json:"evidence"`
}

type WorktreeTrees struct {
	Index    string `json:"index"`
	Worktree string `json:"worktree"`
}

var ErrQuestionClaimed = errors.New("another worker process already claimed this checkpoint answer")

func (s *Store) AskAtCheckpoint(ctx context.Context, execution int64, r CheckpointQuestion) (string, error) {
	if r.Sequence == nil || *r.Sequence < 0 || int64(*r.Sequence) > 2147483647 || len(r.Lease) != 64 || r.Session == "" || len(r.Session) > 128 || !decisionText(r.Question) || !decisionText(r.Evidence) {
		return "", ErrConflict
	}
	var trees any
	if r.ResumeInstance != "" && (r.Trees == nil || len(r.ResumeInstance) != 64 || !validCommit(r.ResumeInstance)) {
		return "", ErrConflict
	}
	if r.Trees != nil {
		if !validCommit(r.Trees.Index) || !validCommit(r.Trees.Worktree) || len(r.Trees.Index) != len(r.Trees.Worktree) {
			return "", ErrConflict
		}
		encoded, err := json.Marshal(r.Trees)
		if err != nil {
			return "", err
		}
		trees = string(encoded)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(r.Lease))
	var task int64
	var base, status, requester string
	var revision int
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.spec->>'base_commit',t.status,e.worker,COALESCE(t.plan_revision,0)
		FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.lease_hash=$2 AND e.uses_checkpoints AND (t.status='RUNNING' OR t.waiting_execution_id=e.id)
		AND e.result IS NULL AND e.abandoned_at IS NULL AND e.ci_candidate IS NULL AND e.resource_kind=''
		AND e.owner_cancel_requested_at IS NULL AND e.ci_pause_requested_at IS NULL AND t.source_closure_id IS NULL
		AND COALESCE(t.spec->>'mode','')<>'plan' AND NOT (t.spec ? 'verify_commit') FOR UPDATE OF e,t`,
		execution, hash[:]).Scan(&task, &base, &status, &requester, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrConflict
	}
	if err != nil {
		return "", err
	}
	if r.Trees != nil && len(r.Trees.Index) != len(base) {
		return "", ErrConflict
	}
	// Read the latest checkpoint after acquiring the same execution lock used to advance it.
	var current bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM
		(SELECT sequence,session_id FROM execution_checkpoints WHERE execution_id=$1 ORDER BY sequence DESC LIMIT 1) c
		JOIN executions e ON e.id=$1 WHERE e.lease_until>clock_timestamp() AND c.sequence=$2 AND c.session_id=$3)`,
		execution, *r.Sequence, r.Session).Scan(&current)
	if err != nil {
		return "", err
	}
	if !current {
		return "", ErrConflict
	}
	var answer string
	var same, fenced bool
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT q.id,COALESCE(answer,''),question=$3 AND evidence=$4 AND trees IS NOT DISTINCT FROM $5::jsonb, cr.question_id IS NOT NULL
		FROM decision_questions q LEFT JOIN checkpoint_resumes cr ON cr.question_id=q.id
		WHERE execution_id=$1 AND checkpoint_sequence=$2 AND withdrawn_at IS NULL`,
		execution, *r.Sequence, r.Question, r.Evidence, trees).Scan(&id, &answer, &same, &fenced)
	if err == nil {
		if !same || fenced != (r.ResumeInstance != "") {
			return "", ErrConflict
		}
		if fenced && answer != "" {
			hash := sha256.Sum256([]byte(r.ResumeInstance))
			err = tx.QueryRowContext(ctx, `UPDATE checkpoint_resumes SET claimant_hash=$2,claimed_at=COALESCE(claimed_at,clock_timestamp())
				WHERE question_id=$1 AND (claimant_hash IS NULL OR claimant_hash=$2) RETURNING question_id`, id, hash[:]).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return "", ErrQuestionClaimed
			}
			if err != nil {
				return "", err
			}
		}
		return answer, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if status != "RUNNING" {
		return "", ErrConflict
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO decision_questions(task_id,execution_id,plan_revision,commit_sha,request_key,question,evidence,requester,checkpoint_sequence,trees)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, task, execution, revision, base, "checkpoint:"+strconv.Itoa(*r.Sequence), r.Question, r.Evidence, requester, *r.Sequence, trees).Scan(&id)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='WAITING_INPUT',waiting_execution_id=$2 WHERE id=$1`, task, execution); err != nil {
		return "", err
	}
	if r.ResumeInstance != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO checkpoint_resumes(question_id) VALUES($1)`, id); err != nil {
			return "", err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type,question_id) VALUES($1,$2,'decision_requested',$3)`, task, execution, id); err != nil {
		return "", err
	}
	return "", tx.Commit()
}

func (s *Store) AnswerQuestion(ctx context.Context, id, qID int64, answer string) (Task, error) {
	if !decisionText(answer) {
		return Task{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var status string
	var prior sql.NullString
	var waiting sql.NullInt64
	var runtime, withdrawn, expired bool
	var execution int64
	err = tx.QueryRowContext(ctx, `SELECT status,waiting_execution_id FROM work_items WHERE id=$1 FOR UPDATE`, id).Scan(&status, &waiting)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	err = tx.QueryRowContext(ctx, `SELECT q.answer,q.execution_id,q.checkpoint_sequence IS NOT NULL,q.withdrawn_at IS NOT NULL,e.lease_until<=clock_timestamp()
		FROM decision_questions q JOIN executions e ON e.id=q.execution_id WHERE q.task_id=$1 AND q.id=$2`, id, qID).Scan(&prior, &execution, &runtime, &withdrawn, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	if prior.Valid {
		if prior.String != answer {
			return Task{}, ErrConflict
		}
	} else {
		if status != "WAITING_INPUT" || withdrawn || (runtime && (waiting.Int64 != execution || expired)) {
			return Task{}, ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE decision_questions SET answer=$2,answered_by='owner',answered_at=now() WHERE id=$1`, qID, answer); err != nil {
			return Task{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status=CASE WHEN waiting_execution_id IS NULL THEN 'WAITING_ACCEPTANCE' ELSE 'RUNNING' END,waiting_execution_id=NULL WHERE id=$1`, id); err != nil {
			return Task{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES ($1,$2,'decision_answered')`, id, execution); err != nil {
			return Task{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}
