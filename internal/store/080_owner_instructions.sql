ALTER TABLE execution_checkpoints
    ADD COLUMN supports_queue boolean NOT NULL DEFAULT false,
    ADD COLUMN next_instructions jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(next_instructions)='array');

CREATE VIEW open_instruction_queues AS
SELECT e.id AS execution_id, c.sequence
FROM executions e JOIN work_items t ON t.id=e.task_id
CROSS JOIN LATERAL (
    SELECT sequence, supports_queue, session_id FROM execution_checkpoints
    WHERE execution_id=e.id ORDER BY sequence DESC LIMIT 1
) c
WHERE c.supports_queue AND c.session_id IS NULL AND e.resource_kind='model'
    AND e.ci_candidate IS NULL AND e.result IS NULL AND e.abandoned_at IS NULL
    AND e.lease_until>clock_timestamp() AND e.owner_cancel_requested_at IS NULL
    AND t.source_closure_id IS NULL AND t.status='RUNNING' AND t.kind='codex_issue'
    AND COALESCE(t.spec->>'mode','')<>'plan' AND COALESCE(t.spec->>'verify_commit','')='';
