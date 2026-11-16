package store

import (
	"context"
	"database/sql"
	"errors"
)

type DeliveryRefresh struct {
	ExecutionID int64    `json:"execution_id"`
	OldDelivery Delivery `json:"old_delivery"`
	NewBase     string   `json:"new_base"`
	Candidate   string   `json:"candidate"`
}

// The caller has confirmed the superseded PR closed without a merge. Archive its full identity before replacing the active queue entry.
func (s *Store) QueueRefreshedCandidate(ctx context.Context, id, execID int64, old, base, candidate string) (Task, error) {
	if !validCommit(base) || !validCommit(candidate) || base == candidate || candidate == old {
		return Task{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM work_items WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return Task{}, err
	}
	var known bool
	err = tx.QueryRowContext(ctx, `SELECT new_base=$3 AND candidate=$4 AND old_delivery->>'commit'=$5 FROM delivery_refreshes WHERE task_id=$1 AND execution_id=$2`, id, execID, base, candidate, old).Scan(&known)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Task{}, err
	}
	if err == nil && !known {
		return Task{}, ErrConflict
	}
	if !known {
		if status != "ACCEPTED" {
			return Task{}, ErrConflict
		}
		var archived int64
		err = tx.QueryRowContext(ctx, `WITH old AS (
			DELETE FROM deliveries WHERE task_id=$1 AND execution_id=$2 AND "commit"=$3 AND state='WAITING' AND pr_number>0 RETURNING *)
			INSERT INTO delivery_refreshes(task_id,execution_id,old_delivery,new_base,candidate)
			SELECT task_id,execution_id,to_jsonb(old)-'available_at'-'lease_token',$4,$5 FROM old RETURNING execution_id`, id, execID, old, base, candidate).Scan(&archived)
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrConflict
		}
		if err != nil {
			return Task{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='READY',spec=spec || jsonb_build_object('base_commit',$2::text,'verify_commit',$3::text) WHERE id=$1`, id, base, candidate); err != nil {
			return Task{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES ($1,$2,'delivery_base_refreshed')`, id, execID); err != nil {
			return Task{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}
