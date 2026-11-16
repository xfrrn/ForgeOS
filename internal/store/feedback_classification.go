package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/lib/pq"
)

type FeedbackJob struct {
	ID     int64           `json:"id"`
	Lease  string          `json:"lease"`
	Source json.RawMessage `json:"source"`
}

type FeedbackClassification struct {
	Category  string `json:"category"`
	Rationale string `json:"rationale"`
	Evidence  string `json:"evidence"`
	Session   string `json:"session_id"`
}

var feedbackCategories = []string{"acceptance", "bug", "ui_change", "requirement_change", "new_work_item", "blocked"}

func (s *Store) ClaimFeedback(ctx context.Context, project int64) (*FeedbackJob, error) {
	var j FeedbackJob
	err := s.db.QueryRowContext(ctx, `WITH next AS (
      SELECT f.id,jsonb_build_object('feedback', f.body - 'request_id',
          'case', (SELECT c FROM candidate_test_plan_receipts plan CROSS JOIN LATERAL jsonb_array_elements(plan.plan->'cases') c
                   WHERE plan.id=f.plan_id AND c->>'id'=f.body->>'case_id')) AS source
      FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id WHERE p.project_id=$1
      AND f.classified_at IS NULL AND (f.review_until IS NULL OR f.review_until<=now()) ORDER BY f.id LIMIT 1 FOR UPDATE OF f SKIP LOCKED)
      UPDATE preview_feedback f SET review_lease=gen_random_uuid(),review_until=now()+interval '6 minutes' FROM next
      WHERE f.id=next.id RETURNING f.id,f.review_lease,next.source`, project).Scan(&j.ID, &j.Lease, &j.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &j, err
}

func (s *Store) ClassifyFeedback(ctx context.Context, project, id int64, lease string, c FeedbackClassification) error {
	if !slices.Contains(feedbackCategories, c.Category) || !decisionText(c.Rationale) || !decisionText(c.Evidence) ||
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
	var task, execution int64
	err = tx.QueryRowContext(ctx, `UPDATE preview_feedback f SET classification=$4::jsonb,classified_at=now()
        FROM static_preview_receipts p JOIN executions e ON e.id=p.execution_id
        WHERE f.id=$1 AND f.preview_id=p.id AND p.project_id=$2 AND f.review_lease::text=$3 AND f.review_until>now()
        AND f.classified_at IS NULL AND strpos(f.body->>'note',$5)>0 AND COALESCE(e.result->>'session_id','')<>$6
        RETURNING p.task_id,p.execution_id`, id, project, lease, string(doc), c.Evidence, c.Session).Scan(&task, &execution)
	if errors.Is(err, sql.ErrNoRows) {
		var replay bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
            WHERE f.id=$1 AND p.project_id=$2 AND f.review_lease::text=$3 AND f.classification=$4::jsonb)`, id, project, lease, string(doc)).Scan(&replay)
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
