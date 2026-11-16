CREATE TABLE repository_inventories (
    task_id bigint PRIMARY KEY REFERENCES work_items(id),
    project_id bigint NOT NULL REFERENCES projects(id),
    execution_id bigint NOT NULL REFERENCES executions(id),
    commit_sha text NOT NULL,
    github_origin text NOT NULL,
    repository_id bigint,
    files jsonb NOT NULL,
    scanned_at timestamptz NOT NULL DEFAULT now()
);
