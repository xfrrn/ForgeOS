-- Original planners committed their result, first revision and event in one transaction.
WITH matches AS (
    SELECT v.id,r.revision,count(*) OVER (PARTITION BY r.task_id) AS events
    FROM task_events v
    JOIN executions e ON e.id=v.execution_id AND e.task_id=v.task_id
    JOIN plan_revisions r ON r.task_id=v.task_id AND r.revision=1 AND r.at=v.at AND r.plan=e.result->'plan'
    WHERE v.type='planner_completed'
)
UPDATE task_events v SET plan_revision=m.revision FROM matches m
WHERE v.id=m.id AND v.plan_revision IS NULL AND m.events=1;
