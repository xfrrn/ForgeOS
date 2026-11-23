CREATE TABLE feature_topics (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    client_id uuid NOT NULL,
    title text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    initial_profile uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, id),
    UNIQUE (project_id, client_id)
);
ALTER TABLE project_chat_turns ADD COLUMN feature_id bigint,
    ADD FOREIGN KEY (project_id, feature_id) REFERENCES feature_topics(project_id, id);
CREATE INDEX feature_turn_history ON project_chat_turns(project_id, feature_id, id DESC);
