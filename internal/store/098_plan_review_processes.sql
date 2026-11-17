CREATE TABLE plan_review_processes (
    id bigserial PRIMARY KEY,
    task_id bigint NOT NULL,
    revision integer NOT NULL,
    lease_hash bytea NOT NULL,
    node text NOT NULL,
    process jsonb NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    FOREIGN KEY(task_id,revision) REFERENCES plan_revisions(task_id,revision),
    UNIQUE(task_id,revision,lease_hash)
);
