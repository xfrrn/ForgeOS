CREATE TABLE project_clone_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    directory text NOT NULL
);

CREATE TABLE repository_clone_jobs (
    id bigserial PRIMARY KEY,
    url text NOT NULL UNIQUE CHECK (url <> ''),
    repository text NOT NULL CHECK (repository <> ''),
    directory text NOT NULL CHECK (directory <> ''),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','cloning','done','failed')),
    lease uuid,
    lease_until timestamptz,
    project_id bigint REFERENCES projects(id),
    path text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'cloning') = (lease IS NOT NULL)),
    CHECK ((lease IS NULL) = (lease_until IS NULL)),
    CHECK ((status = 'done') = (project_id IS NOT NULL)),
    CHECK ((status = 'done') = (path <> '')),
    CHECK ((status = 'failed') = (error <> ''))
);

CREATE INDEX repository_clone_queue ON repository_clone_jobs(id) WHERE status IN ('queued','cloning');
CREATE INDEX repository_clone_project ON repository_clone_jobs(project_id,updated_at DESC,id DESC) WHERE status = 'done';
