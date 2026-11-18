ALTER TABLE executions ADD COLUMN native_approvals boolean NOT NULL DEFAULT false;
CREATE TABLE native_command_approvals (
    id bigserial PRIMARY KEY,
    execution_id bigint NOT NULL REFERENCES executions(id),
    instance bigint NOT NULL,
    session_id text NOT NULL,
    turn_id text NOT NULL,
    item_id text NOT NULL,
    command text NOT NULL,
    cwd text NOT NULL,
    base_commit text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    valid_until timestamptz NOT NULL DEFAULT clock_timestamp() + interval '5 minutes',
    decision text CHECK (decision IN ('accept','decline','cancel')),
    decided_at timestamptz,
    native_status text CHECK (native_status IN ('completed','declined','failed')),
    confirmed_at timestamptz,
    UNIQUE (execution_id,instance,session_id,turn_id,item_id)
);
CREATE VIEW live_command_approvals AS
SELECT c.* FROM native_command_approvals c
JOIN open_native_input_turns a USING(execution_id,instance,session_id,turn_id)
WHERE c.valid_until>clock_timestamp();
