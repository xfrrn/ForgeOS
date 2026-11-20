package store

import (
	"context"
	"encoding/json"

	"github.com/lib/pq"
)

type ChatTaskFact struct {
	ID      int64           `json:"id"`
	Title   string          `json:"title"`
	Status  string          `json:"status"`
	Details json.RawMessage `json:"details"`
}

func (s *Store) ChatTaskFacts(ctx context.Context, project int64, ids []int64) ([]ChatTaskFact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id,
		left(COALESCE(NULLIF(t.spec->'issue'->>'title',''),CASE WHEN t.kind='environment_check'
			THEN '本地环境启动检查' ELSE 'Git HEAD 检查' END),160),t.status,
		jsonb_strip_nulls(jsonb_build_object('goal_id',t.goal_id,'issue_number',t.spec->'issue'->'number',
			'active_worker',CASE WHEN t.status='RUNNING' AND e.lease_until>now() THEN e.worker END,
			'execution_lease_expired',CASE WHEN t.status='RUNNING' THEN e.lease_until<=now() END,
			'pending_dependencies',(SELECT jsonb_agg(d) FROM (
				SELECT p.id,p.status FROM work_dependencies w JOIN work_items p ON p.id=w.prerequisite
				WHERE w.task_id=t.id AND p.project_id=$1 AND p.status<>'DONE' ORDER BY p.id LIMIT 10) d),
			'pending_questions',(SELECT jsonb_agg(q) FROM (
				SELECT id,left(question,300) AS question FROM decision_questions
				WHERE task_id=t.id AND answer IS NULL AND withdrawn_at IS NULL ORDER BY id DESC LIMIT 3) q),
			'latest_event',(SELECT to_jsonb(v) FROM (
				SELECT type,at FROM task_events WHERE task_id=t.id ORDER BY id DESC LIMIT 1) v)))
		FROM work_items t
		LEFT JOIN LATERAL (SELECT worker,lease_until FROM executions WHERE task_id=t.id AND abandoned_at IS NULL
			ORDER BY id DESC LIMIT 1) e ON true
		WHERE t.project_id=$1 AND t.id=ANY($2)
		ORDER BY array_position($2::bigint[],t.id)`, project, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var facts []ChatTaskFact
	for rows.Next() {
		var fact ChatTaskFact
		if err := rows.Scan(&fact.ID, &fact.Title, &fact.Status, &fact.Details); err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}
