ALTER TABLE projects ADD COLUMN screenshot_seconds integer NOT NULL DEFAULT 0 CHECK (screenshot_seconds >= 0);
ALTER TABLE feedback_artifacts ADD COLUMN expires_at timestamptz, ADD COLUMN removed_at timestamptz;
CREATE INDEX feedback_artifacts_cleanup ON feedback_artifacts(id)
    WHERE expires_at IS NOT NULL AND removed_at IS NULL;
