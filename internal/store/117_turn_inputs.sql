CREATE TABLE native_turn_inputs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    execution_id bigint NOT NULL REFERENCES executions(id),
    instance bigint NOT NULL,
    session_id text NOT NULL,
    turn_id text NOT NULL,
    key uuid NOT NULL,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    claimed_at timestamptz,
    claim_key text,
    outcome text CHECK (outcome IN ('accepted','rejected')),
    confirmed_at timestamptz,
    UNIQUE (execution_id,key),
    UNIQUE (execution_id,claim_key)
);

CREATE VIEW open_native_input_turns AS
SELECT e.id AS execution_id,(e.observed_process->>'instance')::bigint AS instance,
       e.observed_process->>'session_id' AS session_id,e.observed_process->>'turn_id' AS turn_id
FROM executions e JOIN work_items t ON t.id=e.task_id
WHERE e.observed_process->>'accepts_turn_input'='true' AND e.observed_process->>'harness'='codex'
  AND e.observed_process->>'turn_id' IS NOT NULL AND e.observed_process->>'exited_at' IS NULL
  AND e.result IS NULL AND e.abandoned_at IS NULL AND e.lease_until>clock_timestamp()
  AND t.status='RUNNING' AND t.source_closure_id IS NULL AND e.owner_cancel_requested_at IS NULL
  AND e.resource_kind='model' AND e.resource_step=(e.observed_process->>'resource_step')::bigint
  AND NOT EXISTS (SELECT 1 FROM model_pauses p WHERE p.execution_id=e.id AND p.resource_step=e.resource_step);
