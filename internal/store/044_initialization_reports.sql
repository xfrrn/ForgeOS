CREATE TABLE initialization_reports (
    task_id bigint NOT NULL REFERENCES repository_inventories(task_id),
    revision integer NOT NULL CHECK (revision > 0),
    generated_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    snapshot jsonb NOT NULL,
    PRIMARY KEY (task_id,revision)
);
