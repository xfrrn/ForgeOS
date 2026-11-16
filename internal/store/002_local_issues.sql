ALTER TABLE projects ADD COLUMN github_repository_id bigint CHECK (github_repository_id > 0);

ALTER TABLE work_items
    ADD COLUMN kind text NOT NULL DEFAULT 'git_head',
    ADD COLUMN github_issue_id bigint,
    ADD COLUMN spec jsonb,
    ADD UNIQUE (project_id, github_issue_id),
    ADD CHECK ((kind = 'git_head' AND github_issue_id IS NULL AND spec IS NULL) OR
               (kind = 'codex_issue' AND github_issue_id IS NOT NULL AND spec IS NOT NULL)),
    DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('READY', 'RUNNING', 'DONE', 'FAILED', 'WAITING_ACCEPTANCE'));
