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
	AnalysisTask int64          `json:"analysis_task,omitempty"`
	ShareID      int64          `json:"share_id,omitempty"`
	TaskID       int64          `json:"task_id,omitempty"`
	Revision     int            `json:"revision,omitempty"`
	Lease        string         `json:"lease"`
	Node         string         `json:"node"`
	Process      HarnessProcess `json:"process"`
}

const planRevisionJSON = `(to_jsonb(r)-'review_lease') || jsonb_build_object('review_processes',
 COALESCE((SELECT jsonb_agg(p.process || jsonb_build_object('node',p.node,'attempt',p.id,'observed_at',p.observed_at) ORDER BY p.id)
 FROM management_processes p WHERE p.task_id=r.task_id AND p.revision=r.revision),'[]'::jsonb))`

const managementProcessTarget = `lease_hash=$1 AND ((task_id=$2 AND revision=$3) OR share_id=$4 OR analysis_task=$5)`

func (s *Store) ObserveManagementProcess(ctx context.Context, project int64, report ManagementProcessReport) error {
	p := report.Process
	if !((report.AnalysisTask > 0 && report.ShareID == 0 && report.TaskID == 0 && report.Revision == 0) ||
		(report.AnalysisTask == 0 && report.ShareID > 0 && report.TaskID == 0 && report.Revision == 0) ||
		(report.AnalysisTask == 0 && report.ShareID == 0 && report.TaskID > 0 && report.Revision > 0)) {
		return ErrConflict
	}
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
	if report.AnalysisTask != 0 {
		err = tx.QueryRowContext(ctx, `SELECT a.finished_at IS NULL AND a.lease_until>clock_timestamp() FROM repository_analyses a JOIN repository_inventories i USING(task_id) WHERE a.task_id=$1 AND i.project_id=$2 AND a.lease::text=$3 FOR UPDATE OF a`, report.AnalysisTask, project, report.Lease).Scan(&valid)
	} else if report.ShareID == 0 {
		err = tx.QueryRowContext(ctx, `SELECT r.review IS NULL AND r.review_until>clock_timestamp()
 FROM plan_revisions r JOIN work_items t ON t.id=r.task_id
 WHERE t.project_id=$1 AND r.task_id=$2 AND r.revision=$3 AND r.review_lease::text=$4 FOR UPDATE OF r`,
			project, report.TaskID, report.Revision, report.Lease).Scan(&valid)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT review IS NULL AND review_until>clock_timestamp() FROM knowledge_shares WHERE project_id=$1 AND id=$2 AND review_lease::text=$3 FOR UPDATE`, project, report.ShareID, report.Lease).Scan(&valid)
	}

	if errors.Is(err, sql.ErrNoRows) || (err == nil && !valid) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(report.Lease))
	var raw []byte
	var node string
	err = tx.QueryRowContext(ctx, `SELECT process,node FROM management_processes WHERE `+managementProcessTarget,
		hash[:], report.TaskID, report.Revision, report.ShareID, report.AnalysisTask).Scan(&raw, &node)
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
	conflictTarget := "task_id,revision,lease_hash"
	if report.ShareID != 0 {
		conflictTarget = "share_id,lease_hash"
	}
	if report.AnalysisTask != 0 {
		conflictTarget = "analysis_task,lease_hash"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO management_processes(task_id,revision,lease_hash,node,process,share_id,analysis_task) VALUES(NULLIF($1,0),NULLIF($2,0),$3,$4,$5,NULLIF($6,0),NULLIF($7,0))
 ON CONFLICT(`+conflictTarget+`) DO UPDATE SET process=excluded.process,
 observed_at=CASE WHEN management_processes.process=excluded.process THEN management_processes.observed_at ELSE clock_timestamp() END`,
		report.TaskID, report.Revision, hash[:], report.Node, string(raw), report.ShareID, report.AnalysisTask)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func verifyManagementProcess(ctx context.Context, tx *sql.Tx, target ManagementProcessReport, session string) error {
	hash := sha256.Sum256([]byte(target.Lease))
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT process FROM management_processes WHERE `+managementProcessTarget, hash[:], target.TaskID, target.Revision, target.ShareID, target.AnalysisTask).Scan(&raw)
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
	if p.Exited == nil || p.ExitCode == nil {
		return ErrConflict
	}
	// Failed analysis may record a real nonzero exit, but never an active process.
	if (target.AnalysisTask == 0 || session != "") && (*p.ExitCode != 0 || p.Session != session) {
		return ErrConflict
	}
	return nil
}
