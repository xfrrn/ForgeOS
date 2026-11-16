CREATE TABLE feedback_work_items (
    feedback_id bigint PRIMARY KEY REFERENCES preview_feedback(id),
    task_id bigint NOT NULL REFERENCES work_items(id)
);
CREATE INDEX task_feedback_sources ON feedback_work_items(task_id);

CREATE VIEW dispatchable_feedback AS
SELECT f.id AS feedback_id,p.id AS preview,p.task_id,p.project_id,p.execution_id,p.commit,
       (t.spec->'issue'->>'number')::bigint AS issue_number
FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id JOIN work_items t ON t.id=p.task_id
WHERE f.classified_at IS NOT NULL AND f.classification->>'category'='new_work_item';
