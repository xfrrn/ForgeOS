CREATE TABLE project_chat_turns (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    client_id uuid NOT NULL,
    text text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','completed','failed')),
    lease uuid,
    lease_until timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    answer text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    evidence jsonb NOT NULL DEFAULT '[]',
    citations jsonb NOT NULL DEFAULT '[]',
    UNIQUE (project_id,client_id),
    UNIQUE (project_id,id)
);
CREATE INDEX project_chat_history ON project_chat_turns(project_id,id DESC);
CREATE INDEX project_chat_pending ON project_chat_turns(project_id,id) WHERE status IN ('queued','running');

CREATE TABLE project_chat_actions (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL,
    turn_id bigint NOT NULL UNIQUE,
    kind text NOT NULL CHECK (kind IN ('dispatch_issue','check_head')),
    label text NOT NULL,
    repository text NOT NULL DEFAULT '',
    issue_number bigint NOT NULL DEFAULT 0,
    repository_id bigint NOT NULL DEFAULT 0,
    source_digest text NOT NULL DEFAULT '',
    work_type text NOT NULL DEFAULT '',
    head_retry bigint NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','confirmed','stale')),
    task_id bigint REFERENCES work_items(id),
    FOREIGN KEY (project_id,turn_id) REFERENCES project_chat_turns(project_id,id)
);
