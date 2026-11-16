ALTER TABLE task_events ADD COLUMN question_id bigint REFERENCES decision_questions(id),
    ADD COLUMN plan_revision integer;
UPDATE task_events v SET question_id=q.id FROM decision_questions q
WHERE v.type='decision_requested' AND v.task_id=q.task_id AND v.execution_id=q.execution_id AND v.at=q.at;
UPDATE task_events v SET plan_revision=r.revision FROM plan_revisions r
WHERE v.type='management_reviewed' AND v.task_id=r.task_id AND v.at=r.reviewed_at;

CREATE TABLE owner_notification_reads (
    event_id bigint PRIMARY KEY REFERENCES task_events(id),
    read_at timestamptz NOT NULL DEFAULT now()
);

CREATE VIEW owner_notifications AS
SELECT v.id,v.at,v.task_id,t.project_id,v.execution_id,v.plan_revision,t.status,
    COALESCE(NULLIF(t.spec->'issue'->>'title',''),CASE t.kind WHEN 'environment_check' THEN '本地环境启动检查' ELSE 'Git HEAD 检查' END) AS title,
    CASE v.type WHEN 'decision_requested' THEN 'question' WHEN 'management_reviewed' THEN 'plan'
        WHEN 'execution_completed' THEN 'acceptance' ELSE 'failure' END AS kind,
    CASE v.type WHEN 'decision_requested' THEN t.status='WAITING_INPUT' AND q.id IS NOT NULL AND q.answer IS NULL
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
