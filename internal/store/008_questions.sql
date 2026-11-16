ALTER TABLE work_items DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('PLANNING','ACTIVE','BLOCKED','READY','RUNNING','DONE','FAILED','WAITING_ACCEPTANCE','ACCEPTED','WAITING_INPUT'));

CREATE TABLE decision_questions (
    id bigserial PRIMARY KEY,
    task_id bigint NOT NULL REFERENCES work_items(id),
    execution_id bigint NOT NULL REFERENCES executions(id),
    plan_revision integer NOT NULL,
    commit_sha text NOT NULL,
    request_key text NOT NULL,
    question text NOT NULL,
    evidence text NOT NULL,
    requester text NOT NULL,
    at timestamptz NOT NULL DEFAULT now(),
    answer text,
    answered_by text,
    answered_at timestamptz,
    UNIQUE (execution_id,request_key)
);
CREATE UNIQUE INDEX one_open_decision ON decision_questions(task_id) WHERE answer IS NULL;
