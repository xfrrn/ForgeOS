ALTER TABLE executions ADD COLUMN supports_model_pause boolean NOT NULL DEFAULT false;
ALTER TABLE work_items DROP CONSTRAINT work_items_check1,
    ADD CONSTRAINT waiting_execution_status CHECK (waiting_execution_id IS NULL OR status IN ('WAITING_INPUT','PAUSED'));

CREATE TABLE model_pauses (
    execution_id bigint NOT NULL REFERENCES executions(id),
    resource_step bigint NOT NULL,
    instance bigint NOT NULL CHECK (instance>0),
    session_id text NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    parked_at timestamptz,
    resumed_at timestamptz,
    index_tree text,
    worktree_tree text,
    PRIMARY KEY (execution_id,resource_step),
    UNIQUE (execution_id,instance),
    CHECK ((parked_at IS NULL) = (index_tree IS NULL) AND (parked_at IS NULL) = (worktree_tree IS NULL)),
    CHECK (resumed_at IS NULL OR parked_at IS NOT NULL)
);
