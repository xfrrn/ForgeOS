package store

import (
	"context"
	"encoding/json"
)

// Each operation retains its own immutable record or plan revision and approval endpoint.
func (s *Store) PendingApprovals(ctx context.Context, kind, beforeKind string, beforeID int64, questions bool) (json.RawMessage, error) {
	var page json.RawMessage
	err := s.db.QueryRowContext(ctx, `WITH approvals AS (
		SELECT 'plan'::text AS kind,t.id,t.project_id,r.plan->>'objective' AS title,r.revision,
		       r.review->>'verdict' AS verdict,COALESCE(r.review->>'effective_level',r.minimum_level,'D2') AS level,
		       NULL::bigint AS recipient,NULL::text AS digest,NULL::timestamptz AS valid_until,NULL::bigint AS task_id
		FROM work_items t JOIN LATERAL (SELECT * FROM plan_revisions WHERE task_id=t.id ORDER BY revision DESC LIMIT 1) r ON true
		WHERE t.status='PLANNING' AND r.revision IS DISTINCT FROM t.active_revision
		UNION ALL
		SELECT 'decision',i.id,i.project_id,i.statement,NULL,c.review->>'verdict',c.review->>'level',NULL,NULL,i.valid_until,NULL
		FROM project_decision_index i JOIN project_decisions d ON d.id=i.id
		LEFT JOIN material_curations c ON (c.project_id,c.id)=(d.project_id,d.curation_id)
		WHERE i.status='PROPOSED'
		UNION ALL
		SELECT 'share',id,project_id,title,revision,review->>'verdict',review->>'effective_level',recipient,digest,NULL,NULL
		FROM knowledge_shares WHERE review->>'verdict'='approve' AND published_at IS NULL AND revoked_at IS NULL
		UNION ALL
		SELECT 'question',q.id,t.project_id,q.question,NULL,NULL,q.management_advice->>'level',NULL,NULL,NULL,t.id
		FROM decision_questions q JOIN work_items t ON t.id=q.task_id JOIN executions e ON e.id=q.execution_id
		WHERE $4 AND q.management_advice->>'level' IN ('D2','D3') AND `+liveAdviceQuestion+`
	), pending AS (
		SELECT * FROM approvals WHERE ($1='' OR kind=$1) AND ($2='' OR (kind COLLATE "C",id)<($2,$3))
		ORDER BY kind COLLATE "C" DESC,id DESC LIMIT 31
	), page AS (SELECT * FROM pending ORDER BY kind COLLATE "C" DESC,id DESC LIMIT 30)
	SELECT jsonb_build_object('items',COALESCE(jsonb_agg(jsonb_strip_nulls(to_jsonb(page)) ORDER BY kind COLLATE "C" DESC,id DESC),'[]'),
		'next_before',CASE WHEN (SELECT count(*) FROM pending)>30 THEN
		    (SELECT kind||'.'||id FROM page ORDER BY kind COLLATE "C",id LIMIT 1) END) FROM page`,
		kind, beforeKind, beforeID, questions).Scan(&page)
	return page, err
}
