package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"forgeos/internal/review"
	"github.com/lib/pq"
)

type ReviewJob struct {
	TaskID      int64  `json:"task_id"`
	ExecutionID int64  `json:"execution_id"`
	Repo        string `json:"repo"`
	Base        string `json:"base_commit"`
	Commit      string `json:"commit"`
	Requirement string `json:"requirement"`
	Lease       string `json:"lease"`
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

func (s *Store) ClaimCodeReview(ctx context.Context, project int64) (*ReviewJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, queueReview+` AND t.project_id=$1 AND p.requires_code_review
		AND NOT EXISTS(SELECT 1 FROM code_reviews r WHERE r.execution_id=e.id) ORDER BY t.id LIMIT 1
		ON CONFLICT DO NOTHING`, project)
	if err != nil {
		return nil, err
	}
	var j ReviewJob
	err = tx.QueryRowContext(ctx, `WITH next AS(
		SELECT r.execution_id,p.path FROM code_reviews r JOIN work_items t ON t.id=r.task_id JOIN projects p ON p.id=t.project_id
		WHERE p.id=$1 AND t.status IN ('WAITING_ACCEPTANCE','WAITING_INPUT') AND r.report IS NULL AND r.failed_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM executions e WHERE e.task_id=t.id AND e.id>r.execution_id)
		AND (r.lease_until IS NULL OR r.lease_until<=now()) ORDER BY r.requested_at,r.execution_id LIMIT 1 FOR UPDATE OF r SKIP LOCKED)
		UPDATE code_reviews r SET lease=gen_random_uuid(),lease_until=now()+interval '6 minutes' FROM next n
		WHERE r.execution_id=n.execution_id RETURNING r.task_id,r.execution_id,n.path,r.base_commit,r.commit_sha,r.requirement,r.lease`, project).
		Scan(&j.TaskID, &j.ExecutionID, &j.Repo, &j.Base, &j.Commit, &j.Requirement, &j.Lease)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
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
	var found int64
	err = s.db.QueryRowContext(ctx, `UPDATE code_reviews r SET report=$4::jsonb,completed_at=COALESCE(completed_at,now())
		FROM work_items t,executions e WHERE t.id=r.task_id AND e.id=r.execution_id AND t.project_id=$1
		AND r.execution_id=$2 AND r.lease::text=$3 AND r.commit_sha=$5 AND r.base_commit=$6
		AND r.failed_at IS NULL AND COALESCE(e.result->>'session_id','') <> $7 AND (r.report=$4::jsonb OR (r.report IS NULL AND r.lease_until>now()))
		RETURNING r.execution_id`, project, execution, lease, string(doc), report.Commit, report.Base, report.Session).Scan(&found)
	var constraint *pq.Error
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &constraint) && constraint.Code == "23505") {
		return ErrConflict
	}
	return err
}

func (s *Store) ReviewReceipt(ctx context.Context, task, execution int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(r) FROM code_review_receipts r WHERE task_id=$1 AND execution_id=$2`, task, execution).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) FailCodeReview(ctx context.Context, project, execution int64, lease string) error {
	var found int64
	err := s.db.QueryRowContext(ctx, `UPDATE code_reviews r SET failed_at=COALESCE(r.failed_at,now())
		FROM work_items t WHERE t.id=r.task_id AND t.project_id=$1 AND r.execution_id=$2 AND r.lease::text=$3
		AND r.report IS NULL AND (r.failed_at IS NOT NULL OR r.lease_until>now()) RETURNING r.execution_id`, project, execution, lease).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
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

func canApproveReview(ctx context.Context, tx *sql.Tx, task, execution int64) error {
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT CASE WHEN r.execution_id IS NULL THEN NOT p.requires_code_review
		ELSE r.report IS NOT NULL AND (r.owner_checked_at IS NOT NULL OR NOT EXISTS(
			SELECT 1 FROM jsonb_array_elements(r.report->'findings') f WHERE (f->>'blocking')::boolean OR NOT (f->>'evidence_verified')::boolean))
		END FROM work_items t JOIN projects p ON p.id=t.project_id LEFT JOIN code_reviews r ON r.task_id=t.id AND r.execution_id=$2
		WHERE t.id=$1 FOR SHARE OF p`, task, execution).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrConflict
	}
	return nil
}
