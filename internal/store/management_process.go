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
	ResumedFrom    int64 `json:"resumed_from,omitempty"`
	MissingSession bool  `json:"missing_session,omitempty"`
	ObservedProcess
	Attempt int64  `json:"attempt"`
	Node    string `json:"node"`
}

type ManagementProcessReport struct {
	IssueClassificationID int64          `json:"issue_classification_id,omitempty"`
	ResumedFrom           int64          `json:"resumed_from,omitempty"`
	MissingSession        bool           `json:"missing_session,omitempty"`
	ReviewerExecution     int64          `json:"reviewer_execution,omitempty"`
	TestPlanID            int64          `json:"test_plan_id,omitempty"`
	TestPlanRole          string         `json:"test_plan_role,omitempty"`
	CurationID            int64          `json:"curation_id,omitempty"`
	CurationRole          string         `json:"curation_role,omitempty"`
	Turn                  int            `json:"turn,omitempty"`
	FeedbackID            int64          `json:"feedback_id,omitempty"`
	QuestionID            int64          `json:"question_id,omitempty"`
	AnalysisTask          int64          `json:"analysis_task,omitempty"`
	ShareID               int64          `json:"share_id,omitempty"`
	TaskID                int64          `json:"task_id,omitempty"`
	Revision              int            `json:"revision,omitempty"`
	Lease                 string         `json:"lease"`
	Node                  string         `json:"node"`
	Process               HarnessProcess `json:"process"`
}

const managementProcessHistoryJSON = `jsonb_agg(p.process || jsonb_strip_nulls(jsonb_build_object('node',p.node,'attempt',p.id,'observed_at',p.observed_at,'resumed_from',NULLIF(p.resumed_from,0),'missing_session',NULLIF(p.missing_session,false))) ORDER BY p.id)`

const planRevisionJSON = `(to_jsonb(r)-'review_lease') || jsonb_build_object('review_processes',
 COALESCE((SELECT ` + managementProcessHistoryJSON + `
 FROM management_processes p WHERE p.task_id=r.task_id AND p.revision=r.revision),'[]'::jsonb))`

const managementProcessTarget = `lease_hash=$1 AND ((task_id=$2 AND revision=$3) OR share_id=$4 OR analysis_task=$5 OR question_id=$6 OR feedback_id=$7 OR (curation_id=$8 AND curation_role=$9 AND ($10=0 OR role_turn=$10)) OR (test_plan_id=$11 AND test_plan_role=$12) OR reviewer_execution=$13 OR issue_classification_id=$14)`

func (s *Store) ObserveManagementProcess(ctx context.Context, project int64, report ManagementProcessReport) error {
	return s.recordRoleProcess(ctx, project, report, nil)
}

func (s *Store) RegisterRoleArchive(ctx context.Context, project int64, report ManagementProcessReport) (int64, error) {
	var id int64
	err := s.recordRoleProcess(ctx, project, report, &id)
	return id, err
}

func (s *Store) recordRoleProcess(ctx context.Context, project int64, report ManagementProcessReport, upload *int64) error {
	p := report.Process
	targets := 0
	for _, id := range []int64{report.TaskID, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.TestPlanID, report.ReviewerExecution, report.IssueClassificationID} {
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
	if (report.TestPlanID == 0 && report.TestPlanRole != "") || (report.TestPlanID != 0 && report.TestPlanRole != "test" && report.TestPlanRole != "management") {
		return ErrConflict
	}
	if report.CurationID == 0 {
		if report.CurationRole != "" || report.Turn != 0 {
			return ErrConflict
		}
	} else if (report.CurationRole != "curator" && report.CurationRole != "management") || report.Turn < 1 || report.Turn > 2 || (report.CurationRole == "management" && report.Turn != 1) {
		return ErrConflict
	}
	harness := "codex"
	if report.ReviewerExecution != 0 {
		harness = "claude"
	}
	if !p.Valid() || (p.Archive != nil && p.Archive.Central) || report.ResumedFrom < 0 || len(p.Tools) != 0 || p.ToolOutput != nil ||
		!p.outputExcludes(report.Lease) || len(report.Lease) != 36 || report.Node == "" || len(report.Node) > 128 ||
		strings.Trim(report.Node, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-") != "" {
		return ErrConflict
	}
	if upload != nil {
		if p.Archive == nil {
			return ErrConflict
		}
		manifest := *p.Archive
		manifest.Central = true
		p.Archive = &manifest
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(report.Lease))
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE project_id=$1 AND role_lease_hash=$2),$3::text)", project, hash[:], harness).Scan(&harness); err != nil {
		return err
	}
	if p.Harness != harness {
		return ErrConflict
	}
	var valid bool
	if report.ReviewerExecution != 0 {
		err = tx.QueryRowContext(ctx, `SELECT r.report IS NULL AND r.failed_at IS NULL AND r.lease_until>clock_timestamp()
 FROM code_reviews r JOIN work_items t ON t.id=r.task_id WHERE r.execution_id=$1 AND t.project_id=$2 AND r.lease::text=$3 FOR UPDATE OF r`, report.ReviewerExecution, project, report.Lease).Scan(&valid)
	} else if report.TestPlanID != 0 {
		// Superseded revisions still report their actual exit before releasing a budget slot.
		err = tx.QueryRowContext(ctx, `SELECT j.completed_at IS NULL AND j.lease_until>clock_timestamp()
 FROM candidate_test_plan_jobs j JOIN candidate_test_plans p ON p.id=j.plan_id JOIN work_items t ON t.id=p.task_id
 WHERE p.id=$1 AND t.project_id=$2 AND j.role=$3 AND j.lease::text=$4 FOR UPDATE OF j`, report.TestPlanID, project, report.TestPlanRole, report.Lease).Scan(&valid)
	} else if report.CurationID != 0 {
		pending, lease, until := "extraction IS NULL", "extraction_lease", "extraction_until"
		if report.CurationRole == "management" {
			pending, lease, until = "extraction IS NOT NULL AND jsonb_array_length(extraction->'facts')>0 AND review IS NULL", "review_lease", "review_until"
		}
		err = tx.QueryRowContext(ctx, `SELECT `+pending+` AND `+until+`>clock_timestamp() FROM material_curations
 WHERE id=$1 AND project_id=$2 AND `+lease+`::text=$3 FOR UPDATE`, report.CurationID, project, report.Lease).Scan(&valid)
	} else if report.IssueClassificationID != 0 {
		err = tx.QueryRowContext(ctx, `SELECT suggestion IS NULL AND lease_until>clock_timestamp() FROM issue_classifications
  WHERE id=$1 AND project_id=$2 AND lease::text=$3 FOR UPDATE`, report.IssueClassificationID, project, report.Lease).Scan(&valid)
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
		if upload == nil {
			return ErrConflict
		}
		// An accepted exit may be acknowledged again after its business lease has expired.
		raw, err := json.Marshal(p)
		if err != nil {
			return err
		}
		err = tx.QueryRowContext(ctx, `SELECT p.id FROM management_processes p JOIN role_archive_transfers a ON a.process_id=p.id
            WHERE `+managementProcessTarget+` AND p.instance=$15 AND a.project_id=$16 AND p.node=$17 AND p.resumed_from=$18
            AND p.missing_session=$19 AND p.process=$20::jsonb`,
			hash[:], report.TaskID, report.Revision, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID,
			report.CurationRole, report.Turn, report.TestPlanID, report.TestPlanRole, report.ReviewerExecution, report.IssueClassificationID,
			p.Instance, project, report.Node, report.ResumedFrom, report.MissingSession, string(raw)).Scan(upload)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if report.CurationID != 0 && report.Turn == 2 {
		var first []byte
		var node string
		err = tx.QueryRowContext(ctx, `SELECT process,node FROM management_processes WHERE curation_id=$1 AND curation_role='curator' AND lease_hash=$2 AND role_turn=1 ORDER BY instance DESC LIMIT 1`, report.CurationID, hash[:]).Scan(&first, &node)
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
	var resumeFrom int64
	var missing bool
	err = tx.QueryRowContext(ctx, `SELECT process,node,resumed_from,missing_session FROM management_processes WHERE `+managementProcessTarget+` AND instance=$15`,
		hash[:], report.TaskID, report.Revision, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.CurationRole, report.Turn, report.TestPlanID, report.TestPlanRole, report.ReviewerExecution, report.IssueClassificationID, p.Instance).Scan(&raw, &node, &resumeFrom, &missing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if raw != nil {
		var prior HarnessProcess
		if err = json.Unmarshal(raw, &prior); err != nil {
			return err
		}
		if prior.Archive != nil {
			if p.Archive == nil {
				p.Archive = prior.Archive
			} else {
				want := *prior.Archive
				want.Central = p.Archive.Central
				if *p.Archive != want {
					return ErrConflict
				}
				if prior.Archive.Central {
					p.Archive = prior.Archive
				}
			}
		}
		if node != report.Node || resumeFrom != report.ResumedFrom || (missing && !report.MissingSession) || !p.follows(prior) {
			return ErrConflict
		}
	} else {
		var latest int64
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(max(instance),0) FROM management_processes WHERE `+managementProcessTarget,
			hash[:], report.TaskID, report.Revision, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.CurationRole, report.Turn, report.TestPlanID, report.TestPlanRole, report.ReviewerExecution, report.IssueClassificationID).Scan(&latest)
		if err != nil {
			return err
		}
		if latest != report.ResumedFrom || (latest != 0 && p.Instance <= latest) {
			return ErrConflict
		}
	}
	if report.ResumedFrom != 0 {
		var previousRaw []byte
		var previousNode string
		var previousMissing bool
		var previousFrom int64
		err = tx.QueryRowContext(ctx, `SELECT process,node,missing_session,resumed_from FROM management_processes WHERE `+managementProcessTarget+` AND instance=$15`,
			hash[:], report.TaskID, report.Revision, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.CurationRole, report.Turn, report.TestPlanID, report.TestPlanRole, report.ReviewerExecution, report.IssueClassificationID, report.ResumedFrom).Scan(&previousRaw, &previousNode, &previousMissing, &previousFrom)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		var previous HarnessProcess
		if err = json.Unmarshal(previousRaw, &previous); err != nil {
			return err
		}
		if previousNode != report.Node || previous.Exited == nil || p.Started.Before(*previous.Exited) || p.Instance <= previous.Instance {
			return ErrConflict
		}
		if previousMissing {
			var original string
			err = tx.QueryRowContext(ctx, `SELECT process->>'session_id' FROM management_processes WHERE `+managementProcessTarget+` AND instance=$15`,
				hash[:], report.TaskID, report.Revision, report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.CurationRole, report.Turn, report.TestPlanID, report.TestPlanRole, report.ReviewerExecution, report.IssueClassificationID, previousFrom).Scan(&original)
			if err != nil {
				return err
			}
			if original == "" || p.HandoffFrom != original || p.Session == original || p.Session == "" || report.MissingSession {
				return ErrConflict
			}
		} else {
			if previous.Session == "" || p.HandoffFrom != "" || (p.Session != "" && p.Session != previous.Session) {
				return ErrConflict
			}
			if report.MissingSession && (p.Session != "" || p.Exited == nil || p.ExitCode == nil || *p.ExitCode != 1) {
				return ErrConflict
			}
		}
	} else if p.HandoffFrom != "" || report.MissingSession {
		return ErrConflict
	}
	raw, err = json.Marshal(p)
	if err != nil {
		return err
	}
	conflictTarget := "task_id,revision,lease_hash"
	if report.IssueClassificationID != 0 {
		conflictTarget = "issue_classification_id,lease_hash"
	}
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
	if report.TestPlanID != 0 {
		conflictTarget = "test_plan_id,test_plan_role,lease_hash"
	}
	if report.ReviewerExecution != 0 {
		conflictTarget = "reviewer_execution,lease_hash"
	}
	var processID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO management_processes(task_id,revision,lease_hash,node,process,share_id,analysis_task,question_id,feedback_id,curation_id,curation_role,role_turn,test_plan_id,test_plan_role,reviewer_execution,resumed_from,missing_session,issue_classification_id) VALUES(NULLIF($1,0),NULLIF($2,0),$3,$4,$5,NULLIF($6,0),NULLIF($7,0),NULLIF($8,0),NULLIF($9,0),NULLIF($10,0),NULLIF($11,''),NULLIF($12,0),NULLIF($13,0),NULLIF($14,''),NULLIF($15,0),$16,$17,NULLIF($18,0))
 ON CONFLICT(`+conflictTarget+`,instance) DO UPDATE SET process=excluded.process,missing_session=excluded.missing_session,
 observed_at=CASE WHEN management_processes.process=excluded.process THEN management_processes.observed_at ELSE clock_timestamp() END RETURNING id`,
		report.TaskID, report.Revision, hash[:], report.Node, string(raw), report.ShareID, report.AnalysisTask, report.QuestionID, report.FeedbackID, report.CurationID, report.CurationRole, report.Turn, report.TestPlanID, report.TestPlanRole, report.ReviewerExecution, report.ResumedFrom, report.MissingSession, report.IssueClassificationID).Scan(&processID)
	if err != nil {
		return err
	}
	if upload != nil {
		role := "management"
		switch {
		case report.CurationRole == "curator":
			role = "curator"
		case report.TestPlanRole == "test":
			role = "test-agent"
		case report.ReviewerExecution > 0:
			role = "reviewer"
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO role_archive_transfers(process_id,instance,project_id,role,verified,expires_at)
           VALUES ($1,$2,$3,$4,$5,clock_timestamp()+make_interval(days=>$6)) ON CONFLICT(process_id) DO NOTHING`, processID, p.Instance, project, role, p.Archive.Bytes == 0, s.ProcessArchiveDays)
		if err != nil {
			return err
		}
		*upload = processID
	}
	return tx.Commit()
}

func verifyManagementProcess(ctx context.Context, tx *sql.Tx, target ManagementProcessReport, session string) error {
	hash := sha256.Sum256([]byte(target.Lease))
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT process FROM management_processes WHERE `+managementProcessTarget+` ORDER BY role_turn DESC,instance DESC LIMIT 1`, hash[:], target.TaskID, target.Revision, target.ShareID, target.AnalysisTask, target.QuestionID, target.FeedbackID, target.CurationID, target.CurationRole, target.Turn, target.TestPlanID, target.TestPlanRole, target.ReviewerExecution, target.IssueClassificationID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		if target.IssueClassificationID != 0 {
			return ErrConflict
		}
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
	// Failed role outcomes may record a real nonzero exit, but never an active process.
	if ((target.AnalysisTask == 0 && target.TestPlanID == 0 && target.ReviewerExecution == 0) || session != "") && (*p.ExitCode != 0 || p.Session != session) {
		return ErrConflict
	}
	return nil
}
