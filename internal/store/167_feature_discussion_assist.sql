ALTER TABLE feature_topics
    ADD COLUMN brief text NOT NULL DEFAULT '',
    ADD COLUMN brief_updated_at timestamptz,
    ADD COLUMN brief_source_turn bigint;

ALTER TABLE project_chat_turns
    ADD COLUMN assist_kind text NOT NULL DEFAULT '' CHECK (assist_kind IN ('', 'turn_notes', 'topic_notes', 'brief')),
    ADD COLUMN assist_target_turn bigint REFERENCES project_chat_turns(id);

CREATE INDEX feature_assist_jobs ON project_chat_turns(project_id, feature_id, assist_kind, id DESC)
    WHERE assist_kind <> '';
