package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
)

// The checkpoint binds a finished implementation turn to the commit about to enter CI.
func (s *Store) RecordCICandidate(ctx context.Context, id int64, lease, sha, session string) error {
	if !validCommit(sha) || lease == "" || len(session) > 256 {
		return ErrConflict
	}
	hash := sha256.Sum256([]byte(lease))
	res, err := s.db.ExecContext(ctx, `UPDATE executions e SET ci_candidate=$3
		FROM work_items t WHERE e.id=$1 AND e.task_id=t.id AND e.lease_hash=$2 AND t.kind='codex_issue'
		AND t.status='RUNNING' AND t.source_closure_id IS NULL AND e.owner_cancel_requested_at IS NULL
		AND e.lease_until>clock_timestamp() AND e.abandoned_at IS NULL AND e.result IS NULL
		AND (e.ci_candidate IS NULL OR e.ci_candidate=$3)
		AND COALESCE(t.spec->>'mode','')<>'plan' AND length(t.spec->>'base_commit')=length($3) AND t.spec->>'base_commit'<>$3
		AND (CASE WHEN t.spec ? 'verify_commit' THEN t.spec->>'verify_commit'=$3 AND $4=''
		     ELSE $4<>'' END)
		AND (NOT e.uses_checkpoints OR EXISTS (
			SELECT 1 FROM execution_checkpoints turn JOIN execution_checkpoints next
			ON next.execution_id=turn.execution_id AND next.sequence=turn.sequence+1
			WHERE turn.execution_id=e.id AND turn.session_id=$4 AND next.session_id IS NULL
			AND NOT EXISTS(SELECT 1 FROM checkpoint_comments c WHERE c.execution_id=e.id AND c.sequence=next.sequence)))
		AND NOT EXISTS(SELECT 1 FROM acceptance_reviews r JOIN executions prev ON prev.id=r.execution_id
			WHERE r.task_id=t.id AND r.decision='reject' AND (r."commit"=$3 OR prev.result->>'session_id'=$4))`, id, hash[:], sha, session)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrConflict
	}
	return nil
}

func (s *Store) ControlCI(ctx context.Context, id int64, resume bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var task int64
	var state, sha string
	var paused, resumed, stopped, ended bool
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.status,COALESCE(e.ci_candidate,''), e.ci_pause_requested_at IS NOT NULL,
		e.ci_resumed_at IS NOT NULL, t.source_closure_id IS NOT NULL OR e.owner_cancel_requested_at IS NOT NULL,
		e.result IS NOT NULL OR e.abandoned_at IS NOT NULL
		FROM executions e JOIN work_items t ON t.id=e.task_id WHERE e.id=$1 FOR UPDATE OF e,t`, id).Scan(&task, &state, &sha, &paused, &resumed, &stopped, &ended)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil || (resume && resumed) || (!resume && paused) {
		return err
	}
	if stopped || sha == "" {
		return ErrConflict
	}
	event := "ci_pause_requested"
	if resume {
		if state != "PAUSED" || !paused || !ended {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, `WITH resumed AS (
			UPDATE executions SET ci_resumed_at=now() WHERE id=$1 RETURNING task_id,ci_candidate
		) UPDATE work_items t SET status='READY',spec=jsonb_set(t.spec,'{verify_commit}',to_jsonb(r.ci_candidate))
			FROM resumed r WHERE t.id=r.task_id`, id)
		event = "ci_resumed"
	} else {
		if state != "RUNNING" || ended {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE executions SET ci_pause_requested_at=now() WHERE id=$1`, id)
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO task_events(task_id,execution_id,type) VALUES($1,$2,$3)", task, id, event); err != nil {
		return err
	}
	return tx.Commit()
}
