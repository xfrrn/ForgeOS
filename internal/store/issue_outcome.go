package store

import (
	"context"
	"database/sql"
	"errors"
)

type IssueOutcome struct {
	Event, Task, Execution int64
	Kind, Lease            string
	ExitCode               *int
	ValidationFailed       bool
}

func (s *Store) ClaimIssueOutcome(ctx context.Context, project int64) (*IssueOutcome, error) {
	var v IssueOutcome
	err := s.db.QueryRowContext(ctx, `WITH due AS (
 SELECT f.event_id,v.task_id,v.execution_id,v.type FROM issue_outcome_feedback f
 JOIN task_events v ON v.id=f.event_id JOIN work_items t ON t.id=v.task_id
 WHERE t.project_id=$1 AND f.confirmed_at IS NULL AND f.retry_at<=now()
 ORDER BY f.retry_at,f.event_id LIMIT 1 FOR UPDATE OF f SKIP LOCKED),
 claimed AS (
 UPDATE issue_outcome_feedback f SET lease=gen_random_uuid(),retry_at=now()+interval '2 minutes'
 FROM due WHERE f.event_id=due.event_id
 RETURNING f.lease,due.*)
 SELECT c.event_id,c.task_id,COALESCE(c.execution_id,0),c.type,c.lease,
 (e.result->>'exit_code')::int,COALESCE((e.result->'validation'->>'exit_code')::int<>0,false)
 FROM claimed c LEFT JOIN executions e ON e.id=c.execution_id`, project).Scan(&v.Event, &v.Task, &v.Execution, &v.Kind, &v.Lease, &v.ExitCode, &v.ValidationFailed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) ConfirmIssueOutcome(ctx context.Context, v IssueOutcome) error {
	r, err := s.db.ExecContext(ctx, `UPDATE issue_outcome_feedback SET confirmed_at=now(),lease=NULL
 WHERE event_id=$1 AND lease=$2::uuid AND confirmed_at IS NULL AND retry_at>now()`, v.Event, v.Lease)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
