CREATE TABLE static_previews (
    id text PRIMARY KEY,
    execution_id bigint NOT NULL REFERENCES executions(id),
    directory text NOT NULL,
    port integer NOT NULL CHECK(port BETWEEN 1 AND 65535),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    stopped_at timestamptz
);
CREATE INDEX static_previews_execution ON static_previews(execution_id, created_at DESC);

CREATE VIEW static_preview_receipts AS
SELECT p.id, e.id AS execution_id, e.task_id, t.project_id, e.workspace AS worktree,
       e.result->>'commit' AS commit, p.directory, 'local'::text AS environment,
       'loopback-capability'::text AS access_policy, 'http://p-' || p.id || '.localhost:' || p.port || '/' AS url,
       p.created_at, p.expires_at, p.stopped_at,
       CASE WHEN p.stopped_at IS NOT NULL THEN 'stopped' WHEN now() >= p.expires_at THEN 'expired' ELSE 'active' END AS state
FROM static_previews p JOIN executions e ON e.id=p.execution_id JOIN work_items t ON t.id=e.task_id;
