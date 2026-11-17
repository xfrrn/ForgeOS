package store

import (
	"context"
	"database/sql"
	"errors"
)

var ErrOwnerStop = errors.New("Owner requested execution stop; report its result")

// Keep the running slot until the worker reports or the owner reconciles an expired lease.
func (s *Store) CancelExecution(ctx context.Context, execution int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var task int64
	var requested, running, paused bool
	err = tx.QueryRowContext(ctx, `SELECT t.id,e.owner_cancel_requested_at IS NOT NULL,
		(t.status='RUNNING' OR t.waiting_execution_id IS NOT DISTINCT FROM e.id) AND t.source_closure_id IS NULL AND e.result IS NULL AND e.abandoned_at IS NULL,
		t.status='PAUSED' AND e.ci_pause_requested_at IS NOT NULL AND e.ci_resumed_at IS NULL
		FROM executions e JOIN work_items t ON t.id=e.task_id WHERE e.id=$1 FOR UPDATE OF e,t`, execution).Scan(&task, &requested, &running, &paused)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil || requested {
		return err
	}
	if !running && !paused {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='RUNNING',waiting_execution_id=NULL WHERE id=$1 AND waiting_execution_id=$2`, task, execution); err != nil {
		return err
	}
	event := "owner_cancel_requested"
	if paused {
		if _, err = tx.ExecContext(ctx, "UPDATE work_items SET status='CANCELLED' WHERE id=$1", task); err != nil {
			return err
		}
		event = "owner_cancelled"
	}
	_, err = tx.ExecContext(ctx, `WITH requested AS (
		UPDATE executions SET owner_cancel_requested_at=now() WHERE id=$1 RETURNING id
	) INSERT INTO task_events(task_id,execution_id,type) SELECT $2,id,$3 FROM requested`, execution, task, event)
	if err != nil {
		return err
	}
	return tx.Commit()
}
