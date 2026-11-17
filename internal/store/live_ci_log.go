package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"forgeos/internal/validation"
)

type LiveCILog struct {
	Run        string     `json:"run"`
	Commit     string     `json:"commit"`
	Check      string     `json:"check"`
	Index      int        `json:"index"`
	Bytes      int        `json:"bytes"`
	Text       string     `json:"text"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

type CILogReport struct {
	Lease string    `json:"lease_token"`
	Log   LiveCILog `json:"log"`
}

func (s *Store) ObserveCILog(ctx context.Context, id int64, report CILogReport) error {
	l := report.Log
	if len(report.Lease) != 64 || l.ObservedAt != nil || !validation.FullCommit(l.Commit) || !validation.ValidRunID(l.Run) || !validation.ValidCheckName(l.Check) ||
		l.Index < 0 || l.Index >= 64 || l.Bytes < 0 || l.Bytes > validation.MaxLogBytes || len(l.Text) > validation.LiveLogWindow || (l.Bytes == 0 && l.Text != "") ||
		validation.DisplayLogTail([]byte(l.Text), []string{report.Lease}) != l.Text {
		return ErrConflict
	}
	hash := sha256.Sum256([]byte(report.Lease))
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE executions e SET live_ci_log=CASE WHEN (e.live_ci_log-'observed_at')=$4::jsonb THEN e.live_ci_log
		ELSE $4::jsonb || jsonb_build_object('observed_at',clock_timestamp()) END FROM work_items t
		WHERE t.id=e.task_id AND t.status='RUNNING' AND t.source_closure_id IS NULL AND e.id=$1 AND e.lease_hash=$2 AND e.ci_candidate=$3
		AND e.lease_until>clock_timestamp() AND e.abandoned_at IS NULL AND e.result IS NULL AND e.owner_cancel_requested_at IS NULL AND e.ci_pause_requested_at IS NULL
		AND (e.live_ci_log IS NULL OR e.live_ci_log-'observed_at'=$4::jsonb OR
			(e.live_ci_log->>'run'=$5 AND (
				(e.live_ci_log->>'index')::int<$6 OR
				((e.live_ci_log->>'index')::int=$6 AND e.live_ci_log->>'check'=$7 AND (e.live_ci_log->>'bytes')::int<$8)
			)))`, id, hash[:], l.Commit, string(raw), l.Run, l.Index, l.Check, l.Bytes)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
