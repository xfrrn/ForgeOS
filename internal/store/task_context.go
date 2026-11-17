package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

const taskContextFrom = `FROM work_items t JOIN work_items g ON g.id=COALESCE(t.goal_id,t.id) AND g.project_id=t.project_id
		LEFT JOIN LATERAL (SELECT r.task_id,r.revision,r.observed_at,r.issue FROM requirement_revisions r
			JOIN work_items src ON src.id=r.task_id AND src.project_id=t.project_id WHERE r.id=t.requirement_id) req ON true
		LEFT JOIN LATERAL (SELECT r.task_id,r.revision,r.at,r.git_commit,r.plan FROM plan_revisions r
			WHERE r.task_id=g.id AND (t.goal_id IS NULL OR r.revision=t.plan_revision)
			ORDER BY r.revision DESC LIMIT 1) p ON true
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(jsonb_build_object('id',d.id,'statement',d.statement,'validity',d.validity,
				'valid_until',d.valid_until,'source',d.source,'confirmed_at',d.history->-1->'at','ref',refs.ref) ORDER BY refs.pos) items
			FROM (SELECT ref,min(pos) AS pos FROM jsonb_array_elements(p.plan->'milestones') m
				CROSS JOIN LATERAL jsonb_array_elements(NULLIF(m->'items','null'::jsonb)) i
				CROSS JOIN LATERAL jsonb_array_elements_text(NULLIF(i->'context_refs','null'::jsonb)) WITH ORDINALITY r(ref,pos)
				WHERE i->>'key'=t.plan_key GROUP BY ref) refs
			JOIN project_decision_index d ON d.project_id=t.project_id AND d.status='CONFIRMED'
				AND refs.ref='/projects/' || t.project_id || '/decisions/' || d.id
		) decisions ON true
		LEFT JOIN LATERAL (SELECT jsonb_agg(jsonb_build_object('task_id',c.id,'key',c.plan_key,'status',c.status,
			'pr_url',d.pr_url,'merge_sha',d.merge_sha) ORDER BY c.id) items
			FROM work_dependencies w JOIN work_items c ON c.id=w.prerequisite AND c.project_id=t.project_id
			LEFT JOIN deliveries d ON d.task_id=c.id WHERE w.task_id=t.id) deps ON true
		WHERE t.project_id=$1 AND t.id=$2`

func (s *Store) TaskContext(ctx context.Context, project, task int64) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT jsonb_build_object(
		'project_id',t.project_id,'task_id',t.id,'kind',t.kind,'status',t.status,
		'base_commit',t.spec->'base_commit','issue',t.spec->'issue','project_config_blob',t.spec->'project_config_blob',
		'goal_id',t.goal_id,'plan_key',t.plan_key,'active_revision',g.active_revision,
		'requirement',to_jsonb(req),'plan',to_jsonb(p),'dependencies',COALESCE(deps.items,'[]'::jsonb),
		'decisions',COALESCE(decisions.items,'[]'::jsonb))
		`+taskContextFrom, project, task).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return result, err
}

// Freeze only the assigned child's sources alongside its lease, including the confirmed prerequisite deliveries.
func childContext(ctx context.Context, tx *sql.Tx, project, task int64) (json.RawMessage, error) {
	var result json.RawMessage
	err := tx.QueryRowContext(ctx, `SELECT jsonb_build_object(
		'goal_id',t.goal_id,'plan_revision',p.revision,'plan_key',t.plan_key,
		'requirement',to_jsonb(req),'dependencies',COALESCE(deps.items,'[]'::jsonb),
		'decisions',COALESCE(decisions.items,'[]'::jsonb)) `+taskContextFrom,
		project, task).Scan(&result)
	return result, err
}
