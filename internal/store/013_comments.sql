CREATE TABLE issue_comment_events (
    id bigserial PRIMARY KEY,
    task_id bigint NOT NULL REFERENCES work_items(id),
    comment_id bigint NOT NULL CHECK(comment_id>0),
    updated_at timestamptz NOT NULL,
    body_hash bytea NOT NULL,
    payload jsonb NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(task_id,comment_id,updated_at,body_hash)
);
CREATE INDEX issue_comment_history ON issue_comment_events(task_id,id);
