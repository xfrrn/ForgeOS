package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/lib/pq"
)

func (s *Store) ChatActivities(ctx context.Context, project int64, turns []ChatTurn) error {
	ids := make([]int64, 0)
	byTurn := make([][]int64, len(turns))
	for i, turn := range turns {
		if turn.Action != nil && turn.Action.TaskID > 0 {
			byTurn[i] = append(byTurn[i], turn.Action.TaskID)
		}
		for _, source := range turn.Sources {
			if id := chatSourceTask(source.Key); id > 0 && !slices.Contains(byTurn[i], id) && len(byTurn[i]) < 4 {
				byTurn[i] = append(byTurn[i], id)
			}
		}
		for _, id := range byTurn[i] {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `
	WITH scope AS (
		SELECT parent.id AS root,t.id
		FROM work_items parent JOIN work_items t ON t.project_id=parent.project_id AND (t.id=parent.id OR t.goal_id=parent.id)
		WHERE parent.project_id=$1 AND parent.id=ANY($2)
	), activity AS (
		SELECT s.root,'event:'||v.id AS key,t.id AS task_id,v.at,
			CASE WHEN v.type='planner_completed' THEN 'Planner'
			     WHEN v.type IN ('management_reviewed','management_question_reviewed','management_decision_answered') THEN 'Management'
			     WHEN v.type IN ('plan_activated','acceptance_approved','acceptance_rejected') THEN 'Owner'
			     WHEN v.type='issue_imported' THEN 'Orchestrator'
			     ELSE 'Worker' END AS role,
			CASE v.type
				WHEN 'issue_imported' THEN '已派发 Issue'
				WHEN 'planner_completed' THEN '已提交任务拆分与依赖'
				WHEN 'management_reviewed' THEN '已审核计划'
				WHEN 'plan_activated' THEN '已激活计划'
				WHEN 'execution_claimed' THEN '已领取执行'
				WHEN 'execution_completed' THEN '已完成执行'
				WHEN 'execution_failed' THEN '执行失败'
				WHEN 'decision_requested' THEN '等待 Owner 答复'
				WHEN 'management_question_reviewed' THEN '已审阅待答问题'
				WHEN 'management_decision_answered' THEN '已依据项目决策答复'
				WHEN 'acceptance_approved' THEN '已通过人工验收'
				WHEN 'acceptance_rejected' THEN '人工验收已退回'
				END AS label,
			CASE WHEN v.type IN ('planner_completed','management_reviewed','plan_activated') AND v.plan_revision IS NOT NULL
				THEN '/plans/'||v.plan_revision
				WHEN v.question_id IS NOT NULL THEN '/questions/'||v.question_id
				WHEN v.execution_id IS NOT NULL THEN '?execution='||v.execution_id ELSE '' END AS suffix
		FROM scope s JOIN work_items t ON t.id=s.id JOIN task_events v ON v.task_id=t.id
		WHERE v.type IN ('issue_imported','planner_completed','management_reviewed','plan_activated',
			'execution_claimed','execution_completed','execution_failed','decision_requested','management_question_reviewed',
			'management_decision_answered','acceptance_approved','acceptance_rejected')
		  AND (v.type NOT IN ('execution_claimed','execution_completed','execution_failed') OR t.spec->>'mode' IS DISTINCT FROM 'plan'
		       OR t.goal_id IS NOT NULL OR EXISTS (SELECT 1 FROM executions x WHERE x.id=v.execution_id AND x.result->>'commit' IS NOT NULL))
		UNION ALL
		SELECT s.root,'review:'||r.execution_id,s.id,COALESCE(r.completed_at,r.failed_at),'Reviewer',
			CASE WHEN r.completed_at IS NOT NULL THEN '已提交候选代码审查' ELSE '候选代码审查失败' END,
			'?execution='||r.execution_id
		FROM scope s JOIN code_reviews r ON r.task_id=s.id WHERE COALESCE(r.completed_at,r.failed_at) IS NOT NULL
		UNION ALL
		SELECT s.root,'validation:'||e.id,s.id,v.at,'CI Runner',
			CASE e.result->'validation'->>'exit_code' WHEN '0' THEN '候选检查通过' ELSE '候选检查未通过' END,'?execution='||e.id
		FROM scope s JOIN executions e ON e.task_id=s.id
		JOIN LATERAL (SELECT min(at) AS at FROM task_events WHERE execution_id=e.id AND type IN ('execution_completed','execution_failed')) v ON v.at IS NOT NULL
		WHERE e.result->'validation'->>'exit_code' IS NOT NULL
		UNION ALL
		SELECT s.root,'test-plan:'||p.id||':'||j.role,s.id,j.completed_at,
			CASE WHEN j.role='test' THEN 'Test Agent' ELSE 'Management' END,
			CASE WHEN j.role='test' THEN '已拟定候选测试计划' ELSE '已审核测试覆盖' END,
			'?execution='||p.execution_id
		FROM scope s JOIN candidate_test_plans p ON p.task_id=s.id JOIN candidate_test_plan_jobs j ON j.plan_id=p.id
		WHERE j.outcome IS NOT NULL AND j.completed_at IS NOT NULL
		UNION ALL
		SELECT s.root,'curator:'||c.id,s.id,c.extracted_at,'Curator','已提取交付经验',''
		FROM scope s JOIN task_events e ON e.task_id=s.id JOIN material_curations c ON c.event_id=e.id AND c.project_id=$1
		WHERE c.extracted_at IS NOT NULL
		UNION ALL
		SELECT s.root,'curation-review:'||c.id,s.id,c.reviewed_at,'Management','已审核交付经验',''
		FROM scope s JOIN task_events e ON e.task_id=s.id JOIN material_curations c ON c.event_id=e.id AND c.project_id=$1
		WHERE c.reviewed_at IS NOT NULL
	), ranked AS (
		SELECT *, row_number() OVER (PARTITION BY root ORDER BY at DESC,key DESC) AS n FROM activity
	)
	SELECT root,key,task_id,at,role,label,suffix FROM ranked WHERE n<=24 ORDER BY at,key`, project, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	byTask := make(map[int64][]ChatActivity)
	for rows.Next() {
		var root, task int64
		var suffix string
		var item ChatActivity
		if err := rows.Scan(&root, &item.Key, &task, &item.At, &item.Role, &item.Label, &suffix); err != nil {
			return err
		}
		item.Path = fmt.Sprintf("/tasks/%d%s", task, suffix)
		item.Label = fmt.Sprintf("T-%d · %s", task, item.Label)
		byTask[root] = append(byTask[root], item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i, ids := range byTurn {
		seen := make(map[string]bool)
		for _, id := range ids {
			for _, activity := range byTask[id] {
				if !seen[activity.Key] {
					turns[i].Activities = append(turns[i].Activities, activity)
					seen[activity.Key] = true
				}
			}
		}
		slices.SortStableFunc(turns[i].Activities, func(a, b ChatActivity) int { return a.At.Compare(b.At) })
	}
	return nil
}
