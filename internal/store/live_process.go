package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"forgeos/internal/validation"
)

type ObservedProcess struct {
	HarnessProcess
	ObservedAt time.Time `json:"observed_at"`
}

type ProcessReport struct {
	LeaseToken string         `json:"lease_token"`
	Process    HarnessProcess `json:"process"`
}

func verifyWriterHandoff(ctx context.Context, tx *sql.Tx, execution int64, p HarnessProcess, source *HarnessProcess) error {
	var sourceInstance int64
	if source != nil {
		sourceInstance = source.Instance
	}
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM writer_handoff_sources h
		WHERE h.execution_id=$1 AND h.source_session=$2 AND h.source_instance<$3 AND ($4::bigint=0 OR h.source_instance=$4)
		AND (h.missing_instance IS NULL OR h.missing_instance<$3)
		AND (h.question_id IS NULL OR $6)
		AND ((h.handoff_session=$5 AND h.handoff_instance=$3)
		 OR (h.handoff_session IS NULL AND ((h.question_id IS NULL AND h.missing_instance IS NULL) OR EXISTS(
		    SELECT 1 FROM execution_checkpoints c WHERE c.execution_id=$1 AND c.sequence=h.sequence AND c.session_id IS NULL))
		    AND NOT EXISTS(SELECT 1 FROM writer_handoff_sources used WHERE used.execution_id=$1 AND used.handoff_session IS NOT NULL
		        AND (used.source_session=$2 OR used.handoff_session=$5 OR used.handoff_instance=$3)))))`,
		execution, p.HandoffFrom, p.Instance, sourceInstance, p.Session, source == nil || (source.ExitCode != nil && *source.ExitCode == 0)).Scan(&allowed)
	if err == nil && !allowed {
		return ErrConflict
	}
	return err
}

func (p HarnessProcess) follows(last HarnessProcess) bool {
	if p.Instance != last.Instance {
		return p.Instance > last.Instance
	}
	if last.Output != nil && (p.Output == nil || p.Output.Sequence < last.Output.Sequence ||
		(p.Output.Sequence == last.Output.Sequence && *p.Output != *last.Output)) {
		return false
	}
	if last.Exited != nil && ((p.Output == nil) != (last.Output == nil) || (p.Output != nil && *p.Output != *last.Output)) {
		return false
	}
	if !p.toolsFollow(last) || p.PID != last.PID || p.Harness != last.Harness || p.Version != last.Version || !p.Started.Equal(last.Started) || (last.Session != "" && (p.Session != last.Session || p.HandoffFrom != last.HandoffFrom)) {
		return false
	}
	return last.Exited == nil || (p.Exited != nil && p.Exited.Equal(*last.Exited) && *p.ExitCode == *last.ExitCode)
}

func (s *Store) ObserveProcess(ctx context.Context, execution int64, report ProcessReport) error {
	p := report.Process
	if !p.Valid() || len(report.LeaseToken) != 64 {
		return ErrConflict
	}
	if p.Output != nil && validation.DisplayLogTail([]byte(p.Output.Text), []string{report.LeaseToken}) != p.Output.Text {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(report.LeaseToken))
	var previous []byte
	var planner bool
	err = tx.QueryRowContext(ctx, `SELECT e.observed_process,COALESCE(t.spec->>'mode','')='plan' FROM executions e JOIN work_items t ON t.id=e.task_id
 WHERE e.id=$1 AND e.lease_hash=$2 AND e.result IS NULL AND e.abandoned_at IS NULL AND (t.status='RUNNING' OR (t.waiting_execution_id=e.id AND $4))
 AND t.spec IS NOT NULL AND COALESCE(t.spec->>'verify_commit','')='' AND COALESCE(NULLIF(t.spec->>'harness',''),'codex')=$3
 FOR UPDATE OF e`, execution, hash[:], p.Harness, p.Exited != nil).Scan(&previous, &planner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if !planner && p.HandoffFrom != "" {
		if err := verifyWriterHandoff(ctx, tx, execution, p, nil); err != nil {
			return err
		}
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
	res, err := tx.ExecContext(ctx, `UPDATE executions SET observed_process=$2::jsonb || jsonb_build_object('observed_at',clock_timestamp(),'resource_step',
		CASE WHEN observed_process->>'instance'=$2::jsonb->>'instance' THEN (observed_process->>'resource_step')::bigint
		     WHEN resource_kind='model' THEN resource_step END)
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
