ALTER TABLE projects ADD COLUMN local_ci_retention jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN local_ci_revision bigint REFERENCES retention_revisions(id);
ALTER TABLE retention_revisions DROP CONSTRAINT retention_revisions_scope_check,
    ADD CHECK (scope IN ('ci','local_ci','feedback_screenshot'));
ALTER TABLE ci_archive_files ADD COLUMN local_expired_at timestamptz,
    ADD COLUMN local_revision bigint REFERENCES retention_revisions(id),
    ADD COLUMN local_removed_at timestamptz;
CREATE INDEX ci_local_pending_removal ON ci_archive_files(execution_id,file)
    WHERE local_expired_at IS NOT NULL AND local_removed_at IS NULL;
