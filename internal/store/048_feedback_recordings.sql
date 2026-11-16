ALTER TABLE projects ADD COLUMN recording_seconds integer NOT NULL DEFAULT 0 CHECK (recording_seconds >= 0),
    ADD COLUMN recording_revision bigint REFERENCES retention_revisions(id);
ALTER TABLE retention_revisions DROP CONSTRAINT retention_revisions_scope_check,
    ADD CHECK (scope IN ('ci','local_ci','feedback_screenshot','feedback_recording'));
