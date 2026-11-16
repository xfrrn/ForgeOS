ALTER TABLE work_items DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('PLANNING','READY','RUNNING','DONE','FAILED','WAITING_ACCEPTANCE','ACCEPTED'));

CREATE TABLE plan_revisions (
    task_id bigint NOT NULL REFERENCES work_items(id),
    revision integer NOT NULL CHECK (revision > 0),
    plan jsonb NOT NULL,
    at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, revision)
);
