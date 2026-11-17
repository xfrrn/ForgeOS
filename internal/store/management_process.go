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
	CurationID   int64          `json:"curation_id,omitempty"`
	CurationRole string         `json:"curation_role,omitempty"`
	Turn         int            `json:"turn,omitempty"`
	FeedbackID   int64          `json:"feedback_id,omitempty"`
	QuestionID   int64          `json:"question_id,omitempty"`
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

const managementProcessTarget = `lease_hash=$1 AND ((task_id=$2 AND revision=$3) OR share_id=$4 OR analysis_task=$5 OR question_id=$6 OR feedback_id=$7 OR (curation_id=$8 AND curation_role=$9 AND ($10=0 OR role_turn=$10)))`

func (s *Store) ObserveManagementProcess(ctx context.Context, project int64, report ManagementProcessReport) error {
	p := report.Process
	targets := 0
	for _, id := range []int64{report.TaskID, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID} {
		if id < 0 {
			return ErrConflict
		}
		if id > 0 {
			targets++
		}
	}
	if targets != 1 || report.Revision < 0 || (report.TaskID > 0) != (report.Revision > 0) {
		return ErrConflict
	}
	if report.CurationID == 0 {
		if report.CurationRole != "" || report.Turn != 0 {
			return ErrConflict
		}
	} else if (report.CurationRole != "curator" && report.CurationRole != "management") || report.Turn < 1 || report.Turn > 2 || (report.CurationRole == "management" && report.Turn != 1) {
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
	if report.CurationID != 0 {
		pending, lease, until := "extraction IS NULL", "extraction_lease", "extraction_until"
		if report.CurationRole == "management" {
			pending, lease, until = "extraction IS NOT NULL AND jsonb_array_length(extraction->'facts')>0 AND review IS NULL", "review_lease", "review_until"
		}
		err = tx.QueryRowContext(ctx, `SELECT `+pending+` AND `+until+`>clock_timestamp() FROM material_curations
 WHERE id=$1 AND project_id=$2 AND `+lease+`::text=$3 FOR UPDATE`, report.CurationID, project, report.Lease).Scan(&valid)
	} else if report.FeedbackID != 0 {
		err = tx.QueryRowContext(ctx, `SELECT f.classified_at IS NULL AND f.review_until>clock_timestamp()
 FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
 WHERE f.id=$1 AND p.project_id=$2 AND f.review_lease::text=$3 FOR UPDATE OF f`, report.FeedbackID, project, report.Lease).Scan(&valid)
	} else if report.QuestionID != 0 {
		err = tx.QueryRowContext(ctx, `SELECT q.management_advice IS NULL AND c.lease_until>clock_timestamp()
 FROM decision_questions q JOIN work_items t ON t.id=q.task_id JOIN question_advice_claims c ON c.question_id=q.id
 WHERE q.id=$1 AND t.project_id=$2 AND c.lease::text=$3 FOR UPDATE OF q,c`, report.QuestionID, project, report.Lease).Scan(&valid)
	} else if report.AnalysisTask != 0 {
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
	if report.CurationID != 0 && report.Turn == 2 {
		var first []byte
		var node string
		err = tx.QueryRowContext(ctx, `SELECT process,node FROM management_processes WHERE curation_id=$1 AND curation_role='curator' AND lease_hash=$2 AND role_turn=1`, report.CurationID, hash[:]).Scan(&first, &node)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		var previous HarnessProcess
		if err = json.Unmarshal(first, &previous); err != nil {
			return err
		}
		if previous.Exited == nil || previous.ExitCode == nil || *previous.ExitCode != 0 || node != report.Node || p.Instance <= previous.Instance || p.Started.Before(*previous.Exited) || (p.Session != "" && p.Session == previous.Session) {
			return ErrConflict
		}
	}
	var raw []byte
	var node string
	err = tx.QueryRowContext(ctx, `SELECT process,node FROM management_processes WHERE `+managementProcessTarget,
		hash[:], report.TaskID, report.Revision, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.CurationRole, report.Turn).Scan(&raw, &node)
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
	if report.QuestionID != 0 {
		conflictTarget = "question_id,lease_hash"
	}
	if report.FeedbackID != 0 {
		conflictTarget = "feedback_id,lease_hash"
	}
	if report.CurationID != 0 {
		conflictTarget = "curation_id,curation_role,lease_hash,role_turn"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO management_processes(task_id,revision,lease_hash,node,process,share_id,analysis_task,question_id,feedback_id,curation_id,curation_role,role_turn) VALUES(NULLIF($1,0),NULLIF($2,0),$3,$4,$5,NULLIF($6,0),NULLIF($7,0),NULLIF($8,0),NULLIF($9,0),NULLIF($10,0),NULLIF($11,''),NULLIF($12,0))
 ON CONFLICT(`+conflictTarget+`) DO UPDATE SET process=excluded.process,
 observed_at=CASE WHEN management_processes.process=excluded.process THEN management_processes.observed_at ELSE clock_timestamp() END`,
		report.TaskID, report.Revision, hash[:], report.Node, string(raw), report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.CurationRole, report.Turn)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func verifyManagementProcess(ctx context.Context, tx *sql.Tx, target ManagementProcessReport, session string) error {
	hash := sha256.Sum256([]byte(target.Lease))
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT process FROM management_processes WHERE `+managementProcessTarget+` ORDER BY role_turn DESC LIMIT 1`, hash[:], target.TaskID, target.Revision, target.ShareID, target.AnalysisTask, target.QuestionID, target.FeedbackID, target.CurationID, target.CurationRole, target.Turn).Scan(&raw)
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
