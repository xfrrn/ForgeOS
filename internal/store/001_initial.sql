CREATE TABLE projects (
    id bigserial PRIMARY KEY,
    path text NOT NULL UNIQUE
);

CREATE TABLE work_items (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    status text NOT NULL CHECK (status IN ('READY', 'RUNNING', 'DONE', 'FAILED'))
);
CREATE UNIQUE INDEX one_running_task_per_project ON work_items(project_id)
    WHERE status = 'RUNNING';
CREATE INDEX ready_work_items ON work_items(id) WHERE status = 'READY';

CREATE TABLE executions (
    id bigserial PRIMARY KEY,
    task_id bigint NOT NULL REFERENCES work_items(id),
    worker text NOT NULL,
    workspace text NOT NULL,
    lease_hash bytea NOT NULL,
    lease_until timestamptz NOT NULL,
    abandoned_at timestamptz,
    result jsonb,
    CHECK (abandoned_at IS NULL OR result IS NULL)
);
CREATE INDEX execution_history ON executions(task_id, id DESC);

CREATE TABLE task_events (
    id bigserial PRIMARY KEY,
    task_id bigint NOT NULL REFERENCES work_items(id),
    execution_id bigint REFERENCES executions(id),
    type text NOT NULL,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX task_event_history ON task_events(task_id, id);
