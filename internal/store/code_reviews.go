package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"forgeos/internal/review"
	"github.com/lib/pq"
)

type ReviewJob struct {
	CaptureProcess   bool            `json:"capture_process,omitempty"`
	CaptureProfile   bool            `json:"capture_profile,omitempty"`
	CandidateContext json.RawMessage `json:"candidate_context,omitempty"`
	TaskID           int64           `json:"task_id"`
	ExecutionID      int64           `json:"execution_id"`
	Repo             string          `json:"repo"`
	RepoRelativePath string          `json:"repo_relative_path,omitempty"`
	Base             string          `json:"base_commit"`
	Commit           string          `json:"commit"`
	Requirement      string          `json:"requirement"`
	Lease            string          `json:"lease"`
	LeaseUntil       *time.Time      `json:"lease_until,omitempty"`
}

func (s *Store) SetCodeReviewPolicy(ctx context.Context, id int64, required bool) error {
	var found int64
	err := s.db.QueryRowContext(ctx, `UPDATE projects SET requires_code_review=$2 WHERE id=$1 RETURNING id`, id, required).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Freeze the requirement used by the candidate. The same selection serves automatic and Owner requests.
const queueReview = `INSERT INTO code_reviews(execution_id,task_id,base_commit,commit_sha,requirement)
	SELECT e.id,t.id,e.result->>'base_commit',e.result->>'commit',concat_ws(E'\n',t.spec->'issue'->>'title',t.spec->'issue'->>'body')
	FROM work_items t JOIN projects p ON p.id=t.project_id
	JOIN LATERAL (SELECT id,result FROM executions WHERE task_id=t.id ORDER BY id DESC LIMIT 1) e ON true
	WHERE t.status='WAITING_ACCEPTANCE' AND t.kind='codex_issue' AND e.result->>'exit_code'='0'
	AND (e.result->'validation'->>'exit_code')='0' AND e.result->'validation'->>'commit'=e.result->>'commit' `

func (s *Store) RequestCodeReview(ctx context.Context, task, execution int64, commit string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var found int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM work_items WHERE id=$1 FOR UPDATE`, task).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, queueReview+` AND t.id=$1 AND e.id=$2 AND e.result->>'commit'=$3
		ON CONFLICT(execution_id) DO UPDATE SET failed_at=NULL, lease_until=CASE WHEN code_reviews.failed_at IS NOT NULL THEN NULL ELSE code_reviews.lease_until END RETURNING execution_id`, task, execution, commit).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ClaimCodeReview(ctx context.Context, project int64, supportsContext, supportsDeadline bool) (*ReviewJob, error) {
	tx, err := s.beginRoleClaim(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	pool, available, err := modelSlot(ctx, tx.Tx, "claude", "", "reviewer", project)
	if err != nil || !available || (pool.Valid && !supportsDeadline) {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, queueReview+` AND t.project_id=$1 AND p.requires_code_review
		AND NOT EXISTS(SELECT 1 FROM code_reviews r WHERE r.execution_id=e.id) ORDER BY t.id LIMIT 1
		ON CONFLICT DO NOTHING`, project)
	if err != nil {
		return nil, err
	}
	var j ReviewJob
	var snapshot []byte
	err = tx.QueryRowContext(ctx, `WITH next AS(
		SELECT r.execution_id,p.path,c.context FROM code_reviews r JOIN work_items t ON t.id=r.task_id JOIN projects p ON p.id=t.project_id
		CROSS JOIN LATERAL review_rules_for_execution(r.execution_id) AS c(context)
		WHERE p.id=$1 AND ($2 OR c.context IS NULL) AND t.status IN ('WAITING_ACCEPTANCE','WAITING_INPUT') AND r.report IS NULL AND r.failed_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM executions e WHERE e.task_id=t.id AND e.id>r.execution_id)
		AND (r.lease_until IS NULL OR r.lease_until<=now()) ORDER BY r.requested_at,r.execution_id LIMIT 1 FOR UPDATE OF r SKIP LOCKED)
		UPDATE code_reviews r SET lease=gen_random_uuid(),lease_until=now()+interval '6 minutes', model_budget=$3, candidate_context=n.context FROM next n
		WHERE r.execution_id=n.execution_id RETURNING r.task_id,r.execution_id,n.path,r.base_commit,r.commit_sha,r.requirement,r.lease,r.candidate_context,
		CASE WHEN $4 THEN r.lease_until END`, project, supportsContext, pool, supportsDeadline).
		Scan(&j.TaskID, &j.ExecutionID, &j.Repo, &j.Base, &j.Commit, &j.Requirement, &j.Lease, &snapshot, &j.LeaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	j.CandidateContext = snapshot
	j.CaptureProfile = true
	return &j, tx.Commit()
}

func (s *Store) CodeReviewTarget(ctx context.Context, project, execution int64, lease string) (ReviewJob, error) {
	var j ReviewJob
	err := s.db.QueryRowContext(ctx, `SELECT r.task_id,r.execution_id,p.path,r.base_commit,r.commit_sha FROM code_reviews r
		JOIN work_items t ON t.id=r.task_id JOIN projects p ON p.id=t.project_id
		WHERE p.id=$1 AND r.execution_id=$2 AND r.lease::text=$3 AND r.failed_at IS NULL AND (r.report IS NOT NULL OR r.lease_until>now())`, project, execution, lease).
		Scan(&j.TaskID, &j.ExecutionID, &j.Repo, &j.Base, &j.Commit)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrConflict
	}
	return j, err
}

func (s *Store) CompleteCodeReview(ctx context.Context, project, execution int64, lease string, report review.Report) error {
	doc, err := json.Marshal(report)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var found int64
	err = tx.QueryRowContext(ctx, `SELECT r.execution_id FROM code_reviews r JOIN work_items t ON t.id=r.task_id WHERE t.project_id=$1 AND r.execution_id=$2 AND r.lease::text=$3 FOR UPDATE OF r`, project, execution, lease).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{ReviewerExecution: execution, Lease: lease}, report.Session); err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `UPDATE code_reviews r SET report=$4::jsonb,completed_at=COALESCE(completed_at,now())
		FROM work_items t WHERE t.id=r.task_id AND t.project_id=$1
		AND r.execution_id=$2 AND r.lease::text=$3 AND r.commit_sha=$5 AND r.base_commit=$6
		AND r.failed_at IS NULL AND (r.report=$4::jsonb OR (r.report IS NULL AND r.lease_until>now()
			AND COALESCE((SELECT session_id FROM execution_candidate_identity WHERE id=candidate_implementation_execution($2)),'') <> $7))
		RETURNING r.execution_id`, project, execution, lease, string(doc), report.Commit, report.Base, report.Session).Scan(&found)
	var constraint *pq.Error
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &constraint) && constraint.Code == "23505") {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReviewReceipt(ctx context.Context, task, execution int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(r) || CASE WHEN c.candidate_context IS NULL THEN '{}'::jsonb
		ELSE jsonb_build_object('candidate_context',c.candidate_context) END || jsonb_build_object('processes',COALESCE((SELECT jsonb_agg(p.process || jsonb_strip_nulls(jsonb_build_object('node',p.node,'attempt',p.id,'observed_at',p.observed_at,'resumed_from',NULLIF(p.resumed_from,0),'missing_session',NULLIF(p.missing_session,false))) ORDER BY p.id) FROM management_processes p WHERE p.reviewer_execution=r.execution_id),'[]'::jsonb))
		FROM code_review_receipts r JOIN code_reviews c USING(execution_id) WHERE r.task_id=$1 AND r.execution_id=$2`, task, execution).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) FailCodeReview(ctx context.Context, project, execution int64, lease string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var found int64
	err = tx.QueryRowContext(ctx, `SELECT r.execution_id FROM code_reviews r JOIN work_items t ON t.id=r.task_id WHERE t.project_id=$1 AND r.execution_id=$2 AND r.lease::text=$3 FOR UPDATE OF r`, project, execution, lease).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{ReviewerExecution: execution, Lease: lease}, ""); err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `UPDATE code_reviews r SET failed_at=COALESCE(r.failed_at,now())
		FROM work_items t WHERE t.id=r.task_id AND t.project_id=$1 AND r.execution_id=$2 AND r.lease::text=$3
		AND r.report IS NULL AND (r.failed_at IS NOT NULL OR r.lease_until>now()) RETURNING r.execution_id`, project, execution, lease).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CheckReviewFindings(ctx context.Context, task, execution int64, commit, note string) error {
	if strings.TrimSpace(note) == "" || len(note) > 4096 || strings.ContainsRune(note, 0) {
		return ErrConflict
	}
	var found int64
	err := s.db.QueryRowContext(ctx, `UPDATE code_reviews SET owner_note=$4,owner_checked_at=COALESCE(owner_checked_at,now())
		WHERE task_id=$1 AND execution_id=$2 AND commit_sha=$3 AND report IS NOT NULL AND (owner_note IS NULL OR owner_note=$4)
		RETURNING execution_id`, task, execution, commit, note).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}

// Share the decision state with the task detail so the Owner sees the gate used at approval.
const codeReviewState = `CASE WHEN r.execution_id IS NULL THEN ''
	WHEN r.failed_at IS NOT NULL THEN 'failed' WHEN r.report IS NULL THEN 'pending'
	WHEN r.owner_checked_at IS NULL AND EXISTS (SELECT 1 FROM jsonb_array_elements(r.report->'findings') f
		WHERE (f->>'blocking')::boolean OR NOT (f->>'evidence_verified')::boolean) THEN 'needs_check'
	ELSE 'clear' END`

func canApproveReview(ctx context.Context, tx *sql.Tx, task, execution int64, requiresPlan bool) error {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT CASE WHEN r.execution_id IS NULL THEN NOT p.requires_code_review
		ELSE (`+codeReviewState+`)='clear'
		END AND COALESCE((SELECT execution_state='passed' FROM candidate_test_plan_status WHERE execution_id=$2 ORDER BY revision DESC LIMIT 1),NOT $3)
		FROM work_items t JOIN projects p ON p.id=t.project_id LEFT JOIN code_reviews r ON r.task_id=t.id AND r.execution_id=$2
		WHERE t.id=$1 FOR SHARE OF p`, task, execution, requiresPlan).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrConflict
	}
	return nil
}
