ALTER TABLE work_items DROP CONSTRAINT work_items_check1,
    ADD CONSTRAINT work_item_kind_shape CHECK ((kind IN ('git_head','environment_check') AND spec IS NULL AND github_issue_id IS NULL AND goal_id IS NULL)
        OR (kind='codex_issue' AND spec IS NOT NULL AND ((github_issue_id IS NOT NULL) <> (goal_id IS NOT NULL))));

CREATE TABLE startup_checks (
    task_id bigint PRIMARY KEY REFERENCES work_items(id),
    source_task_id bigint NOT NULL REFERENCES repository_inventories(task_id),
    environment text NOT NULL CHECK (environment IN ('local-mock','local-integrated'))
);
CREATE INDEX startup_check_history ON startup_checks(source_task_id,environment,task_id DESC);
