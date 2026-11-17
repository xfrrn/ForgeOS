package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"
)

type FeedbackJob struct {
	CaptureProcess bool            `json:"capture_process,omitempty"`
	CaptureProfile bool            `json:"capture_profile,omitempty"`
	LeaseUntil     *time.Time      `json:"lease_until,omitempty"`
	ID             int64           `json:"id"`
	Lease          string          `json:"lease"`
	Source         json.RawMessage `json:"source"`
}

type FeedbackClassification struct {
	Context   *json.RawMessage `json:"context,omitempty"`
	Profile   *AgentProfile    `json:"profile,omitempty"`
	Category  string           `json:"category"`
	Rationale string           `json:"rationale"`
	Evidence  string           `json:"evidence"`
	Session   string           `json:"session_id"`
}

var feedbackCategories = []string{"acceptance", "bug", "ui_change", "requirement_change", "new_work_item", "blocked"}

func (s *Store) ClaimFeedback(ctx context.Context, project int64, supportsDeadline, supportsContext bool) (*FeedbackJob, error) {
	var j FeedbackJob
	tx, pool, err := s.beginManagementClaim(ctx, project, "feedback_classification", supportsDeadline)
	if tx == nil {
		return nil, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `WITH next AS (
      SELECT f.id,jsonb_build_object('feedback', f.body - 'request_id',
          'case', (SELECT c FROM candidate_test_plan_receipts plan CROSS JOIN LATERAL jsonb_array_elements(plan.plan->'cases') c
                   WHERE plan.id=f.plan_id AND c->>'id'=f.body->>'case_id')) AS source,
          CASE WHEN $4 THEN jsonb_build_object(
              'candidate',jsonb_build_object('project_id',p.project_id,'task_id',p.task_id,'execution_id',e.id,
                  'base_commit',e.result->>'base_commit','commit',p.commit),
              'requirement',to_jsonb(r),
              'assigned_item',(SELECT jsonb_build_object('goal_id',t.goal_id,'plan_revision',t.plan_revision,'item',item)
                  FROM plan_revisions plan CROSS JOIN LATERAL jsonb_array_elements(plan.plan->'milestones') m
                  CROSS JOIN LATERAL jsonb_array_elements(m->'items') item
                  WHERE plan.task_id=t.goal_id AND plan.revision=t.plan_revision AND item->>'key'=t.plan_key),
              'rules',review_rules_for_execution(e.id)) END AS context
      FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
      JOIN executions e ON e.id=p.execution_id JOIN work_items t ON t.id=p.task_id
      LEFT JOIN requirement_revisions r ON r.id=e.requirement_id WHERE p.project_id=$1
      AND f.classified_at IS NULL AND (f.review_until IS NULL OR f.review_until<=clock_timestamp()) ORDER BY f.id LIMIT 1 FOR UPDATE OF f SKIP LOCKED)
      UPDATE preview_feedback f SET review_lease=gen_random_uuid(),review_until=clock_timestamp()+interval '6 minutes',model_budget=$2,classification_context=next.context FROM next
      WHERE f.id=next.id RETURNING f.id,f.review_lease,next.source || CASE WHEN next.context IS NULL THEN '{}'::jsonb
          ELSE jsonb_build_object('context',next.context) END,CASE WHEN $3 THEN f.review_until END`, project, pool, supportsDeadline, supportsContext).Scan(&j.ID, &j.Lease, &j.Source, &j.LeaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.CaptureProfile = true
	return &j, tx.Commit()
}

func (s *Store) ClassifyFeedback(ctx context.Context, project, id int64, lease string, c FeedbackClassification) error {
	if c.Context != nil || !validStructuredProfile(c.Profile) || !slices.Contains(feedbackCategories, c.Category) || !decisionText(c.Rationale) || !decisionText(c.Evidence) ||
		c.Session == "" || len(c.Session) > 128 || strings.Trim(c.Session, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return ErrConflict
	}
	doc, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked int64
	err = tx.QueryRowContext(ctx, `SELECT f.id FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
 WHERE f.id=$1 AND p.project_id=$2 AND f.review_lease::text=$3 FOR UPDATE OF f`, id, project, lease).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{FeedbackID: id, Lease: lease}, c.Session); err != nil {
		return err
	}
	var task, execution int64
	err = tx.QueryRowContext(ctx, `UPDATE preview_feedback f SET classification=$4::jsonb || CASE WHEN f.classification_context IS NULL THEN '{}'::jsonb
            ELSE jsonb_build_object('context',f.classification_context) END,classified_at=now()
        FROM static_preview_receipts p JOIN executions e ON e.id=p.execution_id
        WHERE f.id=$1 AND f.preview_id=p.id AND p.project_id=$2 AND f.review_lease::text=$3 AND f.review_until>clock_timestamp()
        AND f.classified_at IS NULL AND strpos(f.body->>'note',$5)>0 AND COALESCE(e.result->>'session_id','')<>$6
        AND NOT EXISTS(SELECT 1 FROM execution_candidate_identity impl WHERE impl.id=candidate_implementation_execution(e.id) AND impl.session_id=$6)
        RETURNING p.task_id,p.execution_id`, id, project, lease, string(doc), c.Evidence, c.Session).Scan(&task, &execution)
	if errors.Is(err, sql.ErrNoRows) {
		var replay bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
            WHERE f.id=$1 AND p.project_id=$2 AND f.review_lease::text=$3 AND (f.classification-'context')=$4::jsonb)`, id, project, lease, string(doc)).Scan(&replay)
		if err != nil {
			return err
		}
		if !replay {
			return ErrConflict
		}
		return nil
	}
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES($1,$2,'preview_feedback_classified')`, task, execution)
	if err != nil {
		return err
	}
	return tx.Commit()
}
