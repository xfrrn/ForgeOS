ALTER TABLE project_chat_turns ADD UNIQUE (project_id,feature_id,id);

CREATE TABLE feature_notes (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL,
    feature_id bigint NOT NULL,
    turn_id bigint NOT NULL,
    client_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('agreement','question')),
    body text NOT NULL CHECK (length(trim(body)) BETWEEN 1 AND 1200),
    closed boolean NOT NULL DEFAULT false,
    UNIQUE (project_id,feature_id,client_id),
    FOREIGN KEY (project_id,feature_id,turn_id) REFERENCES project_chat_turns(project_id,feature_id,id)
);
CREATE INDEX feature_note_history ON feature_notes(project_id,feature_id,id);
