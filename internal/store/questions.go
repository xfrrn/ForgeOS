package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type QuestionRequest struct {
	ExecutionID int64  `json:"execution_id"`
	Commit      string `json:"commit_sha"`
	Key         string `json:"request_key"`
	Question    string `json:"question"`
	Evidence    string `json:"evidence"`
}

type DecisionQuestion struct {
	QuestionRequest
	ID           int64      `json:"id"`
	PlanRevision int        `json:"plan_revision"`
	Requester    string     `json:"requester"`
	At           time.Time  `json:"at"`
	Answer       string     `json:"answer,omitempty"`
	AnsweredBy   string     `json:"answered_by,omitempty"`
	AnsweredAt   *time.Time `json:"answered_at,omitempty"`
}

func decisionText(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0) && len(s) <= 4096
}

func (r QuestionRequest) Valid() bool {
	return r.ExecutionID > 0 && validCommit(r.Commit) && planItemKey.MatchString(r.Key) && decisionText(r.Question) && decisionText(r.Evidence)
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
		WHERE task_id=$1 AND execution_id=$2 AND request_key=$6`, id, r.ExecutionID, r.Question, r.Evidence, r.Commit, r.Key).Scan(&same)
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
	err = tx.QueryRowContext(ctx, `INSERT INTO decision_questions(task_id,execution_id,plan_revision,commit_sha,request_key,question,evidence,requester)
		SELECT $1,id,$3,$4,$5,$6,$7,'owner' FROM executions WHERE task_id=$1 AND id=$2
		AND id=(SELECT max(id) FROM executions WHERE task_id=$1) AND result->>'commit'=$4
		RETURNING id`, id, r.ExecutionID, revision, r.Commit, r.Key, r.Question, r.Evidence).Scan(&qID)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrConflict
	}
	if err != nil {
		return Task{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='WAITING_INPUT' WHERE id=$1`, id); err != nil {
		return Task{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES ($1,$2,'decision_requested')`, id, r.ExecutionID); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
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
	var execution int64
	err = tx.QueryRowContext(ctx, `SELECT status FROM work_items WHERE id=$1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	err = tx.QueryRowContext(ctx, `SELECT answer,execution_id FROM decision_questions WHERE task_id=$1 AND id=$2`, id, qID).Scan(&prior, &execution)
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
		if status != "WAITING_INPUT" {
			return Task{}, ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE decision_questions SET answer=$2,answered_by='owner',answered_at=now() WHERE id=$1`, qID, answer); err != nil {
			return Task{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='WAITING_ACCEPTANCE' WHERE id=$1`, id); err != nil {
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
