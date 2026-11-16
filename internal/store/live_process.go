package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type ObservedProcess struct {
	HarnessProcess
	ObservedAt time.Time `json:"observed_at"`
}

type ProcessReport struct {
	LeaseToken string         `json:"lease_token"`
	Process    HarnessProcess `json:"process"`
}

func (p HarnessProcess) follows(last HarnessProcess) bool {
	if p.Instance != last.Instance {
		return p.Instance > last.Instance
	}
	if p.PID != last.PID || p.Harness != last.Harness || p.Version != last.Version || !p.Started.Equal(last.Started) || (last.Session != "" && p.Session != last.Session) {
		return false
	}
	return last.Exited == nil || (p.Exited != nil && p.Exited.Equal(*last.Exited) && *p.ExitCode == *last.ExitCode)
}

func (s *Store) ObserveProcess(ctx context.Context, execution int64, report ProcessReport) error {
	p := report.Process
	if !p.Valid() || len(report.LeaseToken) != 64 {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(report.LeaseToken))
	var previous []byte
	err = tx.QueryRowContext(ctx, `SELECT e.observed_process FROM executions e JOIN work_items t ON t.id=e.task_id
 WHERE e.id=$1 AND e.lease_hash=$2 AND e.result IS NULL AND e.abandoned_at IS NULL AND t.status='RUNNING'
 AND t.spec IS NOT NULL AND COALESCE(t.spec->>'verify_commit','')='' AND COALESCE(NULLIF(t.spec->>'harness',''),'codex')=$3 FOR UPDATE OF e`, execution, hash[:], p.Harness).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if len(previous) > 0 {
		var last HarnessProcess
		if err = json.Unmarshal(previous, &last); err != nil {
			return err
		}
		if !p.follows(last) {
			return ErrConflict
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE executions SET observed_process=$2::jsonb || jsonb_build_object('observed_at',clock_timestamp())
 WHERE id=$1 AND lease_until>clock_timestamp()`, execution, string(raw))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return tx.Commit()
}
