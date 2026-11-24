package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"time"
)

type FeatureTopic struct {
	Stage    string    `json:"delivery_stage,omitempty"`
	Finished int       `json:"finished_steps,omitempty"`
	Total    int       `json:"total_steps,omitempty"`
	Phase    string    `json:"phase"`
	ID       int64     `json:"id"`
	Title    string    `json:"title"`
	Profile  uuid.UUID `json:"profile_id"`
	Updated  time.Time `json:"updated_at"`
}

// Stage is projected from the same task, child executions and acceptance records
// as the delivery receipt. Keep Phase as the stable navigation destination.
const featureFields = `SELECT CASE WHEN task.id IS NOT NULL THEN 'execution'
 WHEN design.proposal_role='design_final' AND design.status='completed' THEN 'handoff'
 WHEN latest.approved_revision IS NOT NULL THEN 'design'
 WHEN EXISTS(SELECT 1 FROM feature_proposal_revisions WHERE proposal_id=latest.id) THEN 'review'
 WHEN latest.id IS NOT NULL THEN 'proposals' ELSE 'discussion' END,
 f.id,f.title,coalesce(last_turn.model_profile_id,f.initial_profile),
 greatest(last_turn.created_at,f.created_at,latest.created_at,latest.approved_at,design.created_at,action.confirmed_at,event.at),
 CASE
 WHEN task.id IS NULL THEN ''
 WHEN delivery.merge_sha<>'' THEN 'merged'
 WHEN task.status='CANCELLED' THEN 'cancelled'
 WHEN task.status='SUPERSEDED' THEN 'superseded'
 WHEN task.status='WAITING_ACCEPTANCE' THEN 'acceptance'
 WHEN task.status='ACCEPTED' AND delivery.task_id IS NOT NULL THEN 'delivery'
 WHEN task.status IN ('ACCEPTED','DONE') THEN 'accepted'
 WHEN task.status='FAILED' AND parts.total>0 THEN 'integration_failed'
 WHEN task.status='FAILED' OR parts.failed THEN 'implementation_failed'
 WHEN task.status='WAITING_INPUT' OR parts.waiting THEN 'waiting_input'
 WHEN task.status='PAUSED' OR parts.paused THEN 'paused'
 WHEN parts.cancelled THEN 'step_cancelled'
 WHEN rejected.decision='reject' AND task.status IN ('READY','RUNNING') AND NOT (task.spec ? 'verify_commit') AND NOT EXISTS(SELECT 1 FROM work_items c JOIN executions e ON e.task_id=c.id WHERE c.feature_parent=task.id AND e.id>rejected.execution_id)
 THEN CASE WHEN task.status='READY' THEN 'rework_queued' ELSE 'reworking' END
 WHEN parts.revising THEN 'reworking'
 WHEN parts.total>0 AND parts.finished=parts.total THEN CASE WHEN task.status='RUNNING' THEN 'integrating' ELSE 'integration_queued' END
 WHEN task.status='RUNNING' OR parts.running OR parts.finished>0 THEN 'implementing'
 ELSE 'implementation_queued' END,
 parts.finished,parts.total
 FROM feature_topics f
 LEFT JOIN LATERAL (SELECT id,approved_revision,created_at,approved_at FROM feature_proposals WHERE feature_id=f.id ORDER BY version DESC LIMIT 1) latest ON true
 LEFT JOIN LATERAL (SELECT id,proposal_role,status,created_at FROM project_chat_turns WHERE proposal_id=latest.id AND design_version IS NOT NULL ORDER BY design_version DESC,id DESC LIMIT 1) design ON true
 LEFT JOIN project_chat_actions action ON action.turn_id=design.id AND action.status='confirmed'
 LEFT JOIN work_items task ON task.id=action.task_id AND task.project_id=f.project_id
 LEFT JOIN deliveries delivery ON delivery.task_id=task.id
 LEFT JOIN LATERAL (SELECT r.decision,r.execution_id FROM acceptance_reviews r JOIN executions e ON e.id=r.execution_id WHERE r.task_id=task.id AND e.requirement_id IS NOT DISTINCT FROM task.requirement_id ORDER BY e.id DESC LIMIT 1) rejected ON true
 LEFT JOIN LATERAL (SELECT count(*) total,count(*) FILTER(WHERE status='DONE') finished,bool_or(status='FAILED') failed,
 bool_or(status='PAUSED') paused,bool_or(status='WAITING_INPUT') waiting,bool_or(status='CANCELLED') cancelled,
 bool_or(status='RUNNING') running,bool_or(status IN ('READY','RUNNING') AND spec->'feature_step' ? 'revision') revising
 FROM work_items WHERE feature_parent=task.id) parts ON true
 LEFT JOIN LATERAL (SELECT max(last_event.at) at FROM
 (SELECT task.id UNION ALL SELECT id FROM work_items WHERE feature_parent=task.id) scope
 CROSS JOIN LATERAL (SELECT at FROM task_events WHERE task_id=scope.id ORDER BY id DESC LIMIT 1) last_event) event ON true
 LEFT JOIN LATERAL (SELECT model_profile_id,created_at FROM project_chat_turns WHERE project_id=f.project_id AND feature_id=f.id AND proposal_id IS NULL AND assist_kind='' ORDER BY id DESC LIMIT 1) last_turn ON true
 WHERE f.project_id=$1`

func (s *Store) Feature(ctx context.Context, project, id int64) (FeatureTopic, error) {
	var f FeatureTopic
	err := s.db.QueryRowContext(ctx, featureFields+" AND f.id=$2", project, id).Scan(&f.Phase, &f.ID, &f.Title, &f.Profile, &f.Updated, &f.Stage, &f.Finished, &f.Total)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return f, err
}
func (s *Store) Features(ctx context.Context, project int64) ([]FeatureTopic, error) {
	rows, err := s.db.QueryContext(ctx, featureFields+" ORDER BY 5 DESC,f.id DESC", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeatureTopic{}
	for rows.Next() {
		var f FeatureTopic
		if err := rows.Scan(&f.Phase, &f.ID, &f.Title, &f.Profile, &f.Updated, &f.Stage, &f.Finished, &f.Total); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Store) CreateFeature(ctx context.Context, project int64, client, profile uuid.UUID, title string) (FeatureTopic, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO feature_topics(project_id,client_id,initial_profile,title)
 SELECT p.id,$2,$3,$4 FROM projects p WHERE p.id=$1 AND (EXISTS(SELECT 1 FROM chat_models WHERE id=$3 AND revision IS NOT NULL) OR EXISTS(SELECT 1 FROM feature_clients WHERE project_id=p.id AND id=$3 AND seen_at>now()-interval '3 minutes'))
 ON CONFLICT(project_id,client_id) DO NOTHING RETURNING id`, project, client, profile, title).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, "SELECT id FROM feature_topics WHERE project_id=$1 AND client_id=$2 AND initial_profile=$3 AND title=$4", project, client, profile, title).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return FeatureTopic{}, ErrConflict
	}
	if err != nil {
		return FeatureTopic{}, err
	}
	return s.Feature(ctx, project, id)
}
