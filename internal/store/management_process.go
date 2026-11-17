package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type ManagementProcess struct {
	ObservedProcess
	Attempt int64  `json:"attempt"`
	Node    string `json:"node"`
}

type ManagementProcessReport struct {
	TaskID   int64          `json:"task_id"`
	Revision int            `json:"revision"`
	Lease    string         `json:"lease"`
	Node     string         `json:"node"`
	Process  HarnessProcess `json:"process"`
}

const planRevisionJSON = `(to_jsonb(r)-'review_lease') || jsonb_build_object('review_processes',
 COALESCE((SELECT jsonb_agg(p.process || jsonb_build_object('node',p.node,'attempt',p.id,'observed_at',p.observed_at) ORDER BY p.id)
 FROM plan_review_processes p WHERE p.task_id=r.task_id AND p.revision=r.revision),'[]'::jsonb))`

func (s *Store) ObserveManagementProcess(ctx context.Context, project int64, report ManagementProcessReport) error {
	p := report.Process
	if !p.Valid() || p.Harness != "codex" || p.HandoffFrom != "" || len(p.Tools) != 0 || p.ToolOutput != nil ||
		!p.outputExcludes(report.Lease) || len(report.Lease) != 36 || report.Node == "" || len(report.Node) > 128 ||
		strings.Trim(report.Node, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-") != "" {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT r.review IS NULL AND r.review_until>clock_timestamp()
 FROM plan_revisions r JOIN work_items t ON t.id=r.task_id
 WHERE t.project_id=$1 AND r.task_id=$2 AND r.revision=$3 AND r.review_lease::text=$4 FOR UPDATE OF r`,
		project, report.TaskID, report.Revision, report.Lease).Scan(&valid)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !valid) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(report.Lease))
	var raw []byte
	var node string
	err = tx.QueryRowContext(ctx, `SELECT process,node FROM plan_review_processes WHERE task_id=$1 AND revision=$2 AND lease_hash=$3`,
		report.TaskID, report.Revision, hash[:]).Scan(&raw, &node)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if raw != nil {
		var prior HarnessProcess
		if err = json.Unmarshal(raw, &prior); err != nil {
			return err
		}
		if node != report.Node || p.Instance != prior.Instance || !p.follows(prior) {
			return ErrConflict
		}
	}
	raw, err = json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO plan_review_processes(task_id,revision,lease_hash,node,process) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(task_id,revision,lease_hash) DO UPDATE SET process=excluded.process,
 observed_at=CASE WHEN plan_review_processes.process=excluded.process THEN plan_review_processes.observed_at ELSE clock_timestamp() END`,
		report.TaskID, report.Revision, hash[:], report.Node, string(raw))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func verifyManagementProcess(ctx context.Context, tx *sql.Tx, task int64, revision int, lease, session string) error {
	hash := sha256.Sum256([]byte(lease))
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT process FROM plan_review_processes WHERE task_id=$1 AND revision=$2 AND lease_hash=$3`, task, revision, hash[:]).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var p HarnessProcess
	if err = json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.Exited == nil || p.ExitCode == nil || *p.ExitCode != 0 || p.Session != session {
		return ErrConflict
	}
	return nil
}
