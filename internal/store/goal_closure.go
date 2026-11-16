package store

import (
	"context"
	"database/sql"
	"errors"
)

type GoalClosure struct {
	ID    int64
	Lease string
}

// An undetailed future milestone still represents work, so its goal stays open.
func (s *Store) ClaimGoalClosure(ctx context.Context, projectID int64) (*GoalClosure, error) {
	var claim GoalClosure
	err := s.db.QueryRowContext(ctx, `WITH due AS (
		SELECT g.id,r.revision FROM work_items g
		CROSS JOIN LATERAL (SELECT revision,plan FROM plan_revisions WHERE task_id=g.id ORDER BY revision DESC LIMIT 1) r
		WHERE g.project_id=$1 AND g.status='ACTIVE' AND g.active_revision=r.revision AND (g.close_after IS NULL OR g.close_after<=now())
		AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(r.plan->'milestones') m
		    WHERE jsonb_array_length(COALESCE(NULLIF(m->'items','null'::jsonb),'[]'))=0)
		AND EXISTS (SELECT 1 FROM work_items c WHERE c.goal_id=g.id)
		AND NOT EXISTS (SELECT 1 FROM work_items c LEFT JOIN deliveries d ON d.task_id=c.id
		    WHERE c.goal_id=g.id AND c.status<>'SUPERSEDED' AND (c.status<>'DONE' OR d.state IS DISTINCT FROM 'DONE'))
		ORDER BY g.id LIMIT 1 FOR UPDATE OF g SKIP LOCKED)
		UPDATE work_items g SET close_after=now()+interval '2 minutes',close_lease=gen_random_uuid()
		FROM due WHERE due.id=g.id AND g.status='ACTIVE' AND g.active_revision=due.revision RETURNING g.id,g.close_lease`, projectID).Scan(&claim.ID, &claim.Lease)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &claim, nil
}

func (s *Store) CompleteGoal(ctx context.Context, c GoalClosure) error {
	var id int64
	err := s.db.QueryRowContext(ctx, `WITH finished AS (
		UPDATE work_items SET status='DONE',close_after=NULL,close_lease=NULL WHERE id=$1 AND status='ACTIVE'
		AND close_lease::text=$2 AND close_after>now() RETURNING id)
		INSERT INTO task_events(task_id,type) SELECT id,'goal_delivered' FROM finished RETURNING task_id`, c.ID, c.Lease).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}
