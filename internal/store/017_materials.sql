CREATE TABLE project_materials (
    project_id bigint NOT NULL REFERENCES projects(id),
    key text NOT NULL,
    revision integer NOT NULL CHECK (revision > 0),
    kind text NOT NULL CHECK (kind IN ('profile','environment','material')),
    title text NOT NULL,
    source text NOT NULL,
    content text NOT NULL,
    content_sha256 text NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (project_id,key,revision)
);
