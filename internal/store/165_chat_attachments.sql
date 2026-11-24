CREATE TABLE project_chat_attachments (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    turn_id bigint,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    mime text NOT NULL CHECK (mime IN ('image/png','image/jpeg','image/webp','image/gif')),
    sha256 text NOT NULL,
    bytes bigint NOT NULL CHECK (bytes > 0),
    storage_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, id),
    FOREIGN KEY (project_id, turn_id) REFERENCES project_chat_turns(project_id, id)
);
CREATE INDEX project_chat_attachments_turn ON project_chat_attachments(project_id, turn_id, id) WHERE turn_id IS NOT NULL;
