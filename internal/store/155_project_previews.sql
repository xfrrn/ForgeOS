ALTER TABLE work_items DROP CONSTRAINT work_item_kind_shape,
    ADD CONSTRAINT work_item_kind_shape CHECK (
        (kind IN ('git_head','environment_check','project_preview') AND spec IS NULL AND github_issue_id IS NULL AND goal_id IS NULL)
        OR (kind='codex_issue' AND spec IS NOT NULL AND ((github_issue_id IS NOT NULL) <> (goal_id IS NOT NULL)))
        OR (kind='planner_discussion' AND spec IS NOT NULL AND github_issue_id IS NULL AND goal_id IS NULL AND spec->>'mode'='plan'));

CREATE TABLE project_previews (
    project_id bigint PRIMARY KEY REFERENCES projects(id),
    preview_key text NOT NULL UNIQUE CHECK (preview_key ~ '^[0-9a-f]{48}$'),
    secret text NOT NULL CHECK (secret ~ '^[0-9a-f]{64}$'),
    worker text NOT NULL,
    latest_task_id bigint REFERENCES work_items(id),
    active_port integer CHECK (active_port BETWEEN 1 AND 65535),
    active_instance text
);

CREATE TABLE project_preview_operations (
    task_id bigint PRIMARY KEY REFERENCES work_items(id),
    project_id bigint NOT NULL REFERENCES project_previews(project_id),
    action text NOT NULL CHECK (action IN ('start','stop','status'))
);

CREATE TABLE project_preview_requests (
    project_id bigint NOT NULL REFERENCES project_previews(project_id),
    request_id uuid NOT NULL,
    task_id bigint NOT NULL REFERENCES project_preview_operations(task_id),
    PRIMARY KEY (project_id, request_id)
);
