package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"forgeos/internal/validation"
)

type ManagementJob struct {
	RepositoryAnalysis *AnalysisJob      `json:"repository_analysis,omitempty"`
	Feedback           *FeedbackJob      `json:"feedback,omitempty"`
	TestPlan           *TestPlanJob      `json:"test_plan,omitempty"`
	Share              *ShareReviewInput `json:"share,omitempty"`
	TaskID             int64             `json:"task_id"`
	ProjectID          int64             `json:"project_id"`
	Revision           int               `json:"revision"`
	Lease              string            `json:"lease"`
	MinimumLevel       string            `json:"minimum_level"`
	Spec               IssueSpec         `json:"spec"`
	Plan               Plan              `json:"plan"`
}

type ManagementReview struct {
	Level             string `json:"level"`
	Verdict           string `json:"verdict"`
	Rationale         string `json:"rationale"`
	Evidence          string `json:"evidence"`
	SessionID         string `json:"session_id"`
	EffectiveLevel    string `json:"effective_level,omitempty"`
	ValidationMinimum string `json:"validation_minimum,omitempty"`
}

func decisionLevel(level string) bool {
	return level == "D1" || level == "D2" || level == "D3"
}

// An Owner can pin the decision floor once for a specific unclaimed draft.
func (s *Store) SetPlanMinimum(ctx context.Context, id int64, revision int, level string) error {
	if !decisionLevel(level) {
		return ErrConflict
	}
	var result int
	err := s.db.QueryRowContext(ctx, `WITH current_goal AS (
		SELECT id,spec FROM work_items WHERE id=$1 AND status='PLANNING' FOR UPDATE)
		UPDATE plan_revisions r SET minimum_level=$3 FROM current_goal t
		WHERE r.task_id=t.id AND r.revision=$2 AND ($3 <> 'D1' OR t.spec->>'validation'='docs')
		AND (r.minimum_level=$3 OR (r.minimum_level IS NULL AND r.review_lease IS NULL AND r.review IS NULL))
		AND r.revision=(SELECT max(revision) FROM plan_revisions WHERE task_id=t.id) RETURNING r.revision`, id, revision, level).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}

func (s *Store) ClaimManagement(ctx context.Context, projectID int64) (*ManagementJob, error) {
	var job ManagementJob
	var spec, plan []byte
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
		SELECT t.id,t.project_id,t.spec,r.revision FROM work_items t
		JOIN plan_revisions r ON r.task_id=t.id WHERE t.project_id=$1 AND t.status='PLANNING'
		AND r.revision=(SELECT max(revision) FROM plan_revisions WHERE task_id=t.id)
		AND r.review IS NULL AND (r.review_until IS NULL OR r.review_until<=now())
		ORDER BY r.at,t.id LIMIT 1 FOR UPDATE OF t,r SKIP LOCKED)
		UPDATE plan_revisions r SET review_lease=gen_random_uuid(),review_until=now()+interval '6 minutes'
		FROM candidate c WHERE r.task_id=c.id AND r.revision=c.revision
		RETURNING c.id,c.project_id,c.spec,r.revision,r.plan,r.review_lease,COALESCE(r.minimum_level,'D2')`, projectID).Scan(
		&job.TaskID, &job.ProjectID, &spec, &job.Revision, &plan, &job.Lease, &job.MinimumLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(spec, &job.Spec); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(plan, &job.Plan); err != nil {
		return nil, err
	}
	return &job, nil
}

func (review ManagementReview) validate() error {
	if !decisionLevel(review.Level) || (review.ValidationMinimum != "" && !validation.ValidLevel(review.ValidationMinimum)) ||
		(review.Verdict != "approve" && review.Verdict != "revise" && review.Verdict != "needs_owner") ||
		strings.TrimSpace(review.Rationale) == "" || len(review.Rationale) > 4096 || strings.TrimSpace(review.Evidence) == "" || len(review.Evidence) > 4096 ||
		review.SessionID == "" || len(review.SessionID) > 128 || strings.Trim(review.SessionID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" || review.EffectiveLevel != "" {
		return ErrConflict
	}
	return nil
}

func (s *Store) CompleteManagement(ctx context.Context, projectID, id int64, revision int, lease string, review ManagementReview) error {
	if err := review.validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, validation string
	err = tx.QueryRowContext(ctx, `SELECT status,spec->>'validation' FROM work_items WHERE id=$1 AND project_id=$2 FOR UPDATE`, id, projectID).Scan(&status, &validation)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	var floor string
	var stored []byte
	var current, valid bool
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(minimum_level,'D2'),review,review_until>now(),
		revision=(SELECT max(revision) FROM plan_revisions WHERE task_id=$1)
		FROM plan_revisions WHERE task_id=$1 AND revision=$2 AND review_lease::text=$3 FOR UPDATE`, id, revision, lease).Scan(&floor, &stored, &valid, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if validation != "docs" {
		floor = max(floor, "D2")
	}
	if review.ValidationMinimum != "" && validation != "auto" {
		return ErrConflict
	}
	review.EffectiveLevel = max(floor, review.Level)
	if stored != nil {
		var previous ManagementReview
		if err = json.Unmarshal(stored, &previous); err != nil {
			return err
		}
		if previous != review {
			return ErrConflict
		}
		return nil
	}
	if !current || !valid || status != "PLANNING" {
		return ErrConflict
	}
	doc, err := json.Marshal(review)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE plan_revisions SET review=$3, reviewed_at=now() WHERE task_id=$1 AND revision=$2`, id, revision, string(doc)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type,plan_revision) VALUES ($1,'management_reviewed',$2)`, id, revision); err != nil {
		return err
	}
	if review.EffectiveLevel == "D1" && review.Verdict == "approve" {
		if err = activatePlan(ctx, tx, id, revision); err != nil {
			return err
		}
	}
	return tx.Commit()
}
