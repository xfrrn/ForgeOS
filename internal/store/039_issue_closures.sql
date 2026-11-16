ALTER TABLE work_items DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('PLANNING','ACTIVE','BLOCKED','READY','RUNNING','DONE','FAILED','WAITING_ACCEPTANCE','ACCEPTED','WAITING_INPUT','SUPERSEDED','CANCELLED'));

CREATE TABLE github_issue_closures (
    task_id bigint PRIMARY KEY REFERENCES work_items(id),
    repository_id bigint NOT NULL,
    issue_id bigint NOT NULL,
    number bigint NOT NULL,
    url text NOT NULL,
    closed_at timestamptz NOT NULL,
    state_reason text NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE work_items ADD COLUMN source_closure_id bigint REFERENCES github_issue_closures(task_id);
