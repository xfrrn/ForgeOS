ALTER TABLE static_previews ADD COLUMN environment text NOT NULL DEFAULT 'local' CHECK (environment IN ('local', 'local-mock'));

CREATE OR REPLACE VIEW static_preview_receipts AS
SELECT p.id, e.id AS execution_id, e.task_id, t.project_id, e.workspace AS worktree,
       e.result->>'commit' AS commit, p.directory, p.environment,
       'loopback-capability'::text AS access_policy, 'http://p-' || p.id || '.localhost:' || p.port || '/' AS url,
       p.created_at, p.expires_at, p.stopped_at,
       CASE WHEN p.stopped_at IS NOT NULL THEN 'stopped' WHEN now() >= p.expires_at THEN 'expired' ELSE 'active' END AS state
FROM static_previews p JOIN executions e ON e.id=p.execution_id JOIN work_items t ON t.id=e.task_id;
