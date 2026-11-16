CREATE TABLE github_webhook_events (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    repository_id bigint NOT NULL,
    delivery_id uuid NOT NULL,
    event text NOT NULL,
    body bytea NOT NULL,
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    received_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id,delivery_id)
);
CREATE INDEX github_project_events ON github_webhook_events(project_id,id DESC);
