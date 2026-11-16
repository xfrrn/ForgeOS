CREATE TABLE retention_revisions (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    scope text NOT NULL CHECK (scope IN ('ci','feedback_screenshot')),
    actor text NOT NULL CHECK (actor IN ('owner','migration')),
    previous_policy jsonb,
    policy jsonb NOT NULL,
    at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX retention_project_history ON retention_revisions(project_id,id DESC);
ALTER TABLE ci_archive_files ADD COLUMN retention_revision bigint REFERENCES retention_revisions(id);
ALTER TABLE feedback_artifacts ADD COLUMN retention_revision bigint REFERENCES retention_revisions(id);
ALTER TABLE projects ADD COLUMN ci_revision bigint REFERENCES retention_revisions(id),
    ADD COLUMN screenshot_revision bigint REFERENCES retention_revisions(id);

-- Preserve the configuration at upgrade without claiming who set it or when.
INSERT INTO retention_revisions(project_id,scope,actor,policy)
SELECT id,'ci','migration',ci_retention FROM projects WHERE ci_retention <> '{}'
UNION ALL
SELECT id,'feedback_screenshot','migration',jsonb_build_object('seconds',screenshot_seconds)
FROM projects WHERE screenshot_seconds>0;

UPDATE projects p SET ci_revision=r.id FROM retention_revisions r WHERE r.project_id=p.id AND r.scope='ci';
UPDATE projects p SET screenshot_revision=r.id FROM retention_revisions r WHERE r.project_id=p.id AND r.scope='feedback_screenshot';

UPDATE feedback_artifacts a SET retention_revision=r.id
FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
JOIN retention_revisions r ON r.project_id=p.project_id AND r.scope='feedback_screenshot'
WHERE a.feedback_id=f.id AND a.removed_at IS NULL AND a.expires_at>statement_timestamp();
