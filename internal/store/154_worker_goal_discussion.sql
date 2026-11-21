ALTER TABLE work_items DROP CONSTRAINT work_item_kind_shape,
    ADD CONSTRAINT work_item_kind_shape CHECK (
        (kind IN ('git_head','environment_check') AND spec IS NULL AND github_issue_id IS NULL AND goal_id IS NULL)
        OR (kind='codex_issue' AND spec IS NOT NULL AND ((github_issue_id IS NOT NULL) <> (goal_id IS NOT NULL)))
        OR (kind='planner_discussion' AND spec IS NOT NULL AND github_issue_id IS NULL AND goal_id IS NULL AND spec->>'mode'='plan'));
ALTER TABLE goal_discussion_turns
    ALTER COLUMN model_id DROP NOT NULL,
    ALTER COLUMN model_revision DROP NOT NULL,
    ALTER COLUMN model_name SET DEFAULT '',
    ADD COLUMN worker_task_id bigint UNIQUE REFERENCES work_items(id);
-- Interrupted model API turns have no Worker execution. Deliver them via the new queue.
UPDATE goal_discussion_turns SET status='queued', lease=NULL,lease_until=NULL
    WHERE status='running';
