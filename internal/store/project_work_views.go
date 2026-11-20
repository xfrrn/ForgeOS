package store

import (
	"context"
	"encoding/json"
	"time"
)

type ExecutionQueueItem struct {
	TaskID       int64      `json:"task_id"`
	Title        string     `json:"title"`
	Status       string     `json:"status"`
	Role         string     `json:"role"`
	ExecutionID  int64      `json:"execution_id,omitempty"`
	Worker       string     `json:"worker,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	LeaseExpired bool       `json:"lease_expired,omitempty"`
	BlockedBy    []int64    `json:"blocked_by,omitempty"`
	HeldByTaskID int64      `json:"held_by_task_id,omitempty"`
	QuestionID   int64      `json:"question_id,omitempty"`
	GoalOnHold   bool       `json:"goal_on_hold,omitempty"`
}

type ProjectRoleQueue struct {
	Role    string `json:"role"`
	Pending int64  `json:"pending"`
	Running int64  `json:"running"`
}

type ProjectExecutionQueue struct {
	Items  []ExecutionQueueItem `json:"items"`
	Counts map[string]int64     `json:"counts"`
	Roles  []ProjectRoleQueue   `json:"roles"`
}

// ProjectExecutionQueue reads the live task and role queues in one snapshot.
// Counts include every matching task. The list shows up to 12 items per status.
func (s *Store) ProjectExecutionQueue(ctx context.Context, project int64) (ProjectExecutionQueue, error) {
	var page ProjectExecutionQueue
	if _, err := s.Project(ctx, project); err != nil {
		return page, err
	}
	var body []byte
	err := s.db.QueryRowContext(ctx, `WITH project_holder AS (
		SELECT id FROM work_items WHERE project_id=$1 AND (status='RUNNING' OR waiting_execution_id IS NOT NULL)
	), live AS (
		SELECT t.id,t.status,row_number() OVER (PARTITION BY t.status ORDER BY t.id) AS position
		FROM work_items t WHERE t.project_id=$1 AND t.status IN ('READY','RUNNING','BLOCKED','WAITING_INPUT','PAUSED')
	), items AS (
		SELECT t.id AS task_id, `+taskSummaryTitle+` AS title,t.status,
			CASE WHEN e.resource_kind='ci' OR t.kind='environment_check' OR t.spec ? 'verify_commit' THEN 'ci'
			     WHEN t.spec->>'mode'='plan' THEN 'planner' ELSE 'worker' END AS role,
			e.id AS execution_id,e.worker,(e.lease_until<=now()) AS lease_expired,
			(SELECT v.at FROM task_events v WHERE v.task_id=t.id ORDER BY v.id DESC LIMIT 1) AS updated_at,
			CASE WHEN t.status='READY' THEN (SELECT id FROM project_holder WHERE id<>t.id) END AS held_by_task_id,
			CASE WHEN t.status='READY' AND t.goal_id IS NOT NULL THEN g.status<>'ACTIVE' END AS goal_on_hold,
			CASE WHEN t.status='WAITING_INPUT' THEN (
				SELECT q.id FROM decision_questions q WHERE q.task_id=t.id AND q.answer IS NULL AND q.withdrawn_at IS NULL
			) END AS question_id,
			CASE WHEN t.status='BLOCKED' THEN ARRAY(
				SELECT prev.id FROM work_dependencies d JOIN work_items prev ON prev.id=d.prerequisite AND prev.project_id=$1
				LEFT JOIN deliveries delivery ON delivery.task_id=prev.id
				WHERE d.task_id=t.id AND (prev.status<>'DONE' OR delivery.state IS DISTINCT FROM 'DONE') ORDER BY prev.id
			) END AS blocked_by
		FROM live l JOIN work_items t ON t.id=l.id
		LEFT JOIN work_items g ON g.id=t.goal_id AND g.project_id=$1
		LEFT JOIN LATERAL (
			SELECT e.id,e.worker,e.resource_kind,e.lease_until FROM executions e WHERE e.task_id=t.id
			AND e.result IS NULL AND e.abandoned_at IS NULL
			AND (t.status='RUNNING' OR e.id=t.waiting_execution_id OR
				(t.status='PAUSED' AND e.ci_pause_requested_at IS NOT NULL)) ORDER BY e.id DESC LIMIT 1
		) e ON true WHERE l.position<=12
	), role_work AS (
		SELECT 'plan_review' AS role,r.review_until AS until_at
		FROM work_items t JOIN plan_revisions r ON r.task_id=t.id
		WHERE t.project_id=$1 AND t.status='PLANNING' AND r.review IS NULL
		AND r.revision=(SELECT max(revision) FROM plan_revisions WHERE task_id=t.id)
		UNION ALL
		SELECT 'code_review',r.lease_until FROM code_reviews r JOIN work_items t ON t.id=r.task_id
		WHERE t.project_id=$1 AND t.status IN ('WAITING_ACCEPTANCE','WAITING_INPUT') AND r.report IS NULL AND r.failed_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM executions e WHERE e.task_id=t.id AND e.id>r.execution_id)
		UNION ALL
		SELECT CASE WHEN j.role='test' THEN 'test_plan' ELSE 'test_coverage' END,j.lease_until
		FROM candidate_test_plan_jobs j JOIN candidate_test_plans p ON p.id=j.plan_id JOIN work_items t ON t.id=p.task_id
		WHERE t.project_id=$1 AND j.completed_at IS NULL AND `+currentTestPlan+`
		UNION ALL
		SELECT 'curator',c.extraction_until FROM material_curations c WHERE c.project_id=$1 AND c.extraction IS NULL
		UNION ALL
		SELECT 'curation_review',c.review_until FROM material_curations c WHERE c.project_id=$1 AND c.review IS NULL
		AND c.extraction IS NOT NULL AND jsonb_array_length(c.extraction->'facts')>0
	), counts AS (
		SELECT status,count(*) AS total FROM live GROUP BY status
	), roles AS (
		SELECT role,count(*) FILTER (WHERE until_at>now()) AS running,
			count(*) FILTER (WHERE until_at IS NULL OR until_at<=now()) AS pending FROM role_work GROUP BY role
	)
	SELECT jsonb_build_object(
		'items',COALESCE((SELECT jsonb_agg(jsonb_strip_nulls(to_jsonb(i)) ORDER BY task_id) FROM items i),'[]'),
		'counts',COALESCE((SELECT jsonb_object_agg(status,total) FROM counts),'{}'),
		'roles',COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY role) FROM roles r),'[]')
	)`, project).Scan(&body)
	if err != nil {
		return page, err
	}
	err = json.Unmarshal(body, &page)
	return page, err
}

type ProjectTestCheck struct {
	TaskID        int64      `json:"task_id"`
	ExecutionID   int64      `json:"execution_id"`
	Title         string     `json:"title"`
	Worker        string     `json:"worker"`
	Commit        string     `json:"commit,omitempty"`
	CIKind        string     `json:"ci_kind,omitempty"`
	CIState       string     `json:"ci_state,omitempty"`
	PlanRevision  int        `json:"plan_revision,omitempty"`
	PlanState     string     `json:"plan_state,omitempty"`
	TestCaseState string     `json:"test_case_state,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

type ProjectTestCheckPage struct {
	Items      []ProjectTestCheck `json:"items"`
	NextBefore int64              `json:"next_before,omitempty"`
}

func (s *Store) ProjectTestChecks(ctx context.Context, project, before int64) (ProjectTestCheckPage, error) {
	var page ProjectTestCheckPage
	if _, err := s.Project(ctx, project); err != nil {
		return page, err
	}
	const limit = 25
	var body []byte
	err := s.db.QueryRowContext(ctx, `WITH matched AS (
		SELECT e.id FROM work_items t JOIN executions e ON e.task_id=t.id
		WHERE t.project_id=$1 AND ($2::bigint=0 OR e.id<$2) AND (
			e.result->'validation' IS NOT NULL OR e.result->'environment_run' IS NOT NULL OR e.ci_candidate IS NOT NULL
			OR e.resource_kind='ci' OR e.live_ci_log IS NOT NULL
			OR EXISTS(SELECT 1 FROM candidate_test_plans p WHERE p.execution_id=e.id AND p.task_id=t.id)
		) ORDER BY e.id DESC LIMIT $3
	), items AS (
		SELECT e.id AS execution_id,t.id AS task_id,`+taskSummaryTitle+` AS title,e.worker,
			COALESCE(e.result#>>'{validation,commit}',e.result#>>'{environment_run,commit}',
				e.live_ci_log->>'commit',e.ci_candidate,e.result->>'commit') AS commit,
			CASE WHEN t.kind='environment_check' THEN 'startup' ELSE 'validation' END AS ci_kind,
			CASE WHEN e.result->'validation' IS NOT NULL THEN
				  CASE WHEN e.result#>>'{validation,exit_code}'='0' THEN 'passed' ELSE 'failed' END
			     WHEN e.result->'environment_run' IS NOT NULL THEN
				  CASE WHEN e.result->>'exit_code'='0' AND COALESCE(e.result#>>'{environment_run,failure}','')='' THEN 'passed' ELSE 'failed' END
			     WHEN e.abandoned_at IS NOT NULL THEN 'interrupted'
			     WHEN e.result IS NOT NULL THEN 'missing_receipt'
			     WHEN e.ci_pause_requested_at IS NOT NULL THEN 'paused'
			     WHEN e.lease_until<=now() THEN 'lease_expired'
			     WHEN e.resource_kind='ci' OR e.live_ci_log IS NOT NULL THEN 'running'
			     WHEN e.ci_candidate IS NOT NULL THEN 'pending' END AS ci_state,
			p.revision AS plan_revision,p.state AS plan_state,p.execution_state AS test_case_state,
			GREATEST(p.requested_at,p.planned_at,p.reviewed_at,e.abandoned_at,
				(e.live_ci_log->>'observed_at')::timestamptz,
				(e.result#>>'{validation,run,finished_at}')::timestamptz,
				(e.result#>>'{environment_run,finished_at}')::timestamptz,
				(SELECT max(recorded_at) FROM candidate_test_case_results WHERE plan_id=p.id),
				(SELECT v.at FROM task_events v WHERE v.task_id=t.id AND v.execution_id=e.id ORDER BY v.id DESC LIMIT 1)
			) AS updated_at
		FROM matched m JOIN executions e ON e.id=m.id JOIN work_items t ON t.id=e.task_id
		LEFT JOIN LATERAL (
			SELECT id,revision,state,execution_state,requested_at,planned_at,reviewed_at FROM candidate_test_plan_status
			WHERE execution_id=e.id AND task_id=t.id ORDER BY revision DESC LIMIT 1
		) p ON true
	)
	SELECT COALESCE(jsonb_agg(jsonb_strip_nulls(to_jsonb(i)) ORDER BY execution_id DESC),'[]') FROM items i`, project, before, limit+1).Scan(&body)
	if err != nil {
		return page, err
	}
	if err := json.Unmarshal(body, &page.Items); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextBefore = page.Items[limit-1].ExecutionID
	}
	return page, nil
}
