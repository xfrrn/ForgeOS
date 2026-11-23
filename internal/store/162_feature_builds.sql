ALTER TABLE project_chat_actions ADD COLUMN implementation_steps jsonb;
ALTER TABLE work_items ADD COLUMN feature_parent bigint REFERENCES work_items(id),
    ADD COLUMN feature_position integer,
    ADD CHECK ((feature_parent IS NULL) = (feature_position IS NULL)),
    ADD UNIQUE(feature_parent, feature_position);
ALTER TABLE work_items DROP CONSTRAINT work_item_kind_shape,
    ADD CONSTRAINT work_item_kind_shape CHECK (
        (kind IN ('git_head','environment_check','project_preview') AND spec IS NULL AND github_issue_id IS NULL AND goal_id IS NULL AND feature_parent IS NULL)
        OR (kind='codex_issue' AND spec IS NOT NULL AND num_nonnulls(github_issue_id,goal_id,feature_parent)=1)
        OR (kind='planner_discussion' AND spec IS NOT NULL AND github_issue_id IS NULL AND goal_id IS NULL AND feature_parent IS NULL AND spec->>'mode'='plan'));
CREATE TABLE feature_builds (
    task_id bigint PRIMARY KEY REFERENCES work_items(id),
    worker text
);
ALTER TABLE executions ADD COLUMN feature_inputs jsonb;
DROP INDEX one_running_task_per_project;
CREATE UNIQUE INDEX one_running_task_per_project ON work_items(project_id)
    WHERE feature_parent IS NULL AND (status='RUNNING' OR waiting_execution_id IS NOT NULL);
