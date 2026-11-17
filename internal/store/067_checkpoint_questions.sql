ALTER TABLE work_items ADD COLUMN waiting_execution_id bigint REFERENCES executions(id),
    ADD CHECK (waiting_execution_id IS NULL OR status='WAITING_INPUT');
DROP INDEX one_running_task_per_project;
CREATE UNIQUE INDEX one_running_task_per_project ON work_items(project_id)
    WHERE status='RUNNING' OR waiting_execution_id IS NOT NULL;

ALTER TABLE decision_questions ADD COLUMN checkpoint_sequence integer,
    ADD FOREIGN KEY (execution_id,checkpoint_sequence) REFERENCES execution_checkpoints(execution_id,sequence),
    ADD UNIQUE (execution_id,checkpoint_sequence),
    ADD COLUMN withdrawn_at timestamptz;
DROP INDEX one_open_decision;
CREATE UNIQUE INDEX one_open_decision ON decision_questions(task_id) WHERE answer IS NULL AND withdrawn_at IS NULL;

CREATE OR REPLACE VIEW owner_notifications AS
SELECT v.id,v.at,v.task_id,t.project_id,v.execution_id,v.plan_revision,t.status,
    COALESCE(NULLIF(t.spec->'issue'->>'title',''),CASE t.kind WHEN 'environment_check' THEN '本地环境启动检查' ELSE 'Git HEAD 检查' END) AS title,
    CASE v.type WHEN 'decision_requested' THEN 'question' WHEN 'management_reviewed' THEN 'plan'
        WHEN 'execution_completed' THEN 'acceptance' ELSE 'failure' END AS kind,
    CASE v.type WHEN 'decision_requested' THEN t.status='WAITING_INPUT' AND q.id IS NOT NULL AND q.answer IS NULL AND q.withdrawn_at IS NULL
        WHEN 'management_reviewed' THEN t.status='PLANNING' AND NOT EXISTS (
            SELECT 1 FROM plan_revisions later WHERE later.task_id=t.id AND later.revision>v.plan_revision)
        ELSE t.status=CASE v.type WHEN 'execution_completed' THEN 'WAITING_ACCEPTANCE' ELSE 'FAILED' END AND
            NOT EXISTS (SELECT 1 FROM executions later WHERE later.task_id=t.id AND later.id>v.execution_id)
        END AS pending,
    a.read_at
FROM task_events v JOIN work_items t ON t.id=v.task_id
LEFT JOIN owner_notification_reads a ON a.event_id=v.id
LEFT JOIN decision_questions q ON q.id=v.question_id
LEFT JOIN plan_revisions r ON r.task_id=t.id AND r.revision=v.plan_revision
WHERE v.type IN ('execution_failed','execution_abandoned','decision_requested')
    OR (v.type='execution_completed' AND t.kind='codex_issue')
    OR (v.type='management_reviewed' AND r.review IS NOT NULL
        AND NOT (r.review->>'effective_level'='D1' AND r.review->>'verdict'='approve'));

-- The implementation turn and independent reviewers must see the same Owner clarification.
CREATE OR REPLACE FUNCTION review_rules_for_execution(target_id bigint) RETURNS jsonb
LANGUAGE sql STABLE AS $$
    SELECT (CASE WHEN answers.content IS NULL THEN c.context
                 ELSE COALESCE(c.context,'{}'::jsonb) || jsonb_build_object('owner_clarifications',answers.content) END)
        || CASE WHEN e.id=target_id THEN '{}'::jsonb ELSE
            jsonb_build_object('implementation_execution_id',e.id,'implementation_commit',e.commit_sha) END
    FROM execution_candidate_identity e JOIN candidate_rule_context c ON c.execution_id=e.id
    LEFT JOIN LATERAL (
        SELECT jsonb_agg(jsonb_build_object('sequence',q.checkpoint_sequence,'question',q.question,'answer',q.answer)
                         ORDER BY q.checkpoint_sequence) AS content
        FROM decision_questions q WHERE q.execution_id=e.id AND q.checkpoint_sequence IS NOT NULL AND q.answer IS NOT NULL
    ) answers ON true
    WHERE e.id=candidate_implementation_execution(target_id)
$$;
