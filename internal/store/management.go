package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"forgeos/internal/validation"
)

type ManagementJob struct {
	IssueClassification *IssueClassificationJob `json:"issue_classification,omitempty"`
	CaptureProcess      bool                    `json:"capture_process,omitempty"`
	ReviewContext       json.RawMessage         `json:"review_context,omitempty"`
	CaptureProfile      bool                    `json:"capture_profile,omitempty"`
	LeaseUntil          *time.Time              `json:"lease_until,omitempty"`
	Question            *QuestionAdviceJob      `json:"question,omitempty"`
	Curation            *CurationJob            `json:"curation,omitempty"`
	RepositoryAnalysis  *AnalysisJob            `json:"repository_analysis,omitempty"`
	Feedback            *FeedbackJob            `json:"feedback,omitempty"`
	TestPlan            *TestPlanJob            `json:"test_plan,omitempty"`
	Share               *ShareReviewInput       `json:"share,omitempty"`
	TaskID              int64                   `json:"task_id"`
	ProjectID           int64                   `json:"project_id"`
	Revision            int                     `json:"revision"`
	Lease               string                  `json:"lease"`
	MinimumLevel        string                  `json:"minimum_level"`
	Spec                IssueSpec               `json:"spec"`
	Plan                Plan                    `json:"plan"`
}

type ManagementReview struct {
	Profile           *AgentProfile `json:"profile,omitempty"`
	Level             string        `json:"level"`
	Verdict           string        `json:"verdict"`
	Rationale         string        `json:"rationale"`
	Evidence          string        `json:"evidence"`
	SessionID         string        `json:"session_id"`
	EffectiveLevel    string        `json:"effective_level,omitempty"`
	ValidationMinimum string        `json:"validation_minimum,omitempty"`
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

func (s *Store) ClaimManagement(ctx context.Context, projectID int64, supportsDeadline, supportsContext bool) (*ManagementJob, error) {
	var job ManagementJob
	var spec, plan, reviewContext []byte
	tx, pool, err := s.beginManagementClaim(ctx, projectID, "plan_review", supportsDeadline)
	if tx == nil {
		return nil, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT t.id,t.project_id,t.spec,r.revision FROM work_items t
		JOIN plan_revisions r ON r.task_id=t.id WHERE t.project_id=$1 AND t.status='PLANNING'
		AND r.revision=(SELECT max(revision) FROM plan_revisions WHERE task_id=t.id)
		AND r.review IS NULL AND (r.review_until IS NULL OR r.review_until<=clock_timestamp())
		ORDER BY r.at,t.id LIMIT 1 FOR UPDATE OF t,r SKIP LOCKED)
		UPDATE plan_revisions r SET review_lease=gen_random_uuid(),review_until=clock_timestamp()+interval '6 minutes',model_budget=$2,
		review_context=CASE WHEN $4 THEN jsonb_build_object(
			'global_rules',(SELECT to_jsonb(g) FROM global_rule_revisions g ORDER BY revision DESC LIMIT 1),
			'decisions',COALESCE((SELECT jsonb_agg(jsonb_build_object(
				'id',d.id,'statement',d.statement,'validity',d.validity,'valid_until',d.valid_until,
				'source',d.source,'confirmed_at',d.history->-1->'at','ref',refs.ref,'plan_keys',refs.keys) ORDER BY d.id)
				FROM (SELECT ref,jsonb_agg(DISTINCT i->>'key' ORDER BY i->>'key') AS keys FROM jsonb_array_elements(r.plan->'milestones') m
					CROSS JOIN LATERAL jsonb_array_elements(NULLIF(m->'items','null'::jsonb)) i
					CROSS JOIN LATERAL jsonb_array_elements_text(NULLIF(i->'context_refs','null'::jsonb)) f(ref)
					GROUP BY ref) refs
				JOIN project_decision_index d ON d.project_id=$1 AND d.status='CONFIRMED'
					AND refs.ref='/projects/' || d.project_id || '/decisions/' || d.id),'[]'::jsonb)) END
		FROM candidate c WHERE r.task_id=c.id AND r.revision=c.revision
		RETURNING c.id,c.project_id,c.spec,r.revision,r.plan,r.review_lease,COALESCE(r.minimum_level,'D2'),CASE WHEN $3 THEN r.review_until END,r.review_context`, projectID, pool, supportsDeadline, supportsContext).Scan(
		&job.TaskID, &job.ProjectID, &spec, &job.Revision, &plan, &job.Lease, &job.MinimumLevel, &job.LeaseUntil, &reviewContext)
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
	job.CaptureProfile = true
	job.ReviewContext = reviewContext
	return &job, tx.Commit()
}

func (review ManagementReview) validate() error {
	if !validStructuredProfile(review.Profile) || !decisionLevel(review.Level) || (review.ValidationMinimum != "" && !validation.ValidLevel(review.ValidationMinimum)) ||
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
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(minimum_level,'D2'),review,review_until>clock_timestamp(),
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
		if !reflect.DeepEqual(previous, review) {
			return ErrConflict
		}
		return nil
	}
	if valid && (!current || status != "PLANNING") {
		// The superseded review has finished running. Its verdict cannot apply to the current draft.
		if _, err = tx.ExecContext(ctx, `UPDATE plan_revisions SET review_until=clock_timestamp() WHERE task_id=$1 AND revision=$2`, id, revision); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrConflict
	}
	if !valid {
		return ErrConflict
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{TaskID: id, Revision: revision, Lease: lease}, review.SessionID); err != nil {
		return err
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
		if err = activatePlan(ctx, tx, id, revision, "management"); err != nil {
			return err
		}
	}
	return tx.Commit()
}
