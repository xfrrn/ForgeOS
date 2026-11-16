ALTER TABLE work_items
    ADD COLUMN goal_id bigint,
    ADD COLUMN plan_revision integer,
    ADD COLUMN plan_key text,
    ADD FOREIGN KEY (goal_id,plan_revision) REFERENCES plan_revisions(task_id,revision) MATCH FULL,
    ADD UNIQUE (goal_id,plan_key),
    ADD CHECK ((goal_id IS NULL) = (plan_key IS NULL)),
    DROP CONSTRAINT work_items_check,
    ADD CHECK ((kind='git_head' AND spec IS NULL AND github_issue_id IS NULL AND goal_id IS NULL)
        OR (kind='codex_issue' AND spec IS NOT NULL AND ((github_issue_id IS NOT NULL) <> (goal_id IS NOT NULL)))),
    DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('PLANNING','ACTIVE','BLOCKED','READY','RUNNING','DONE','FAILED','WAITING_ACCEPTANCE','ACCEPTED'));

CREATE TABLE work_dependencies (
    task_id bigint NOT NULL REFERENCES work_items(id),
    prerequisite bigint NOT NULL REFERENCES work_items(id),
    PRIMARY KEY (task_id,prerequisite),
    CHECK (task_id <> prerequisite)
);
