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
	var requested, running bool
	err = tx.QueryRowContext(ctx, `SELECT t.id,e.owner_cancel_requested_at IS NOT NULL,
		t.status='RUNNING' AND t.source_closure_id IS NULL AND e.result IS NULL AND e.abandoned_at IS NULL
		FROM executions e JOIN work_items t ON t.id=e.task_id WHERE e.id=$1 FOR UPDATE OF e,t`, execution).Scan(&task, &requested, &running)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil || requested {
		return err
	}
	if !running {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `WITH requested AS (
		UPDATE executions SET owner_cancel_requested_at=now() WHERE id=$1 RETURNING id
	) INSERT INTO task_events(task_id,execution_id,type) SELECT $2,id,'owner_cancel_requested' FROM requested`, execution, task)
	if err != nil {
		return err
	}
	return tx.Commit()
}
