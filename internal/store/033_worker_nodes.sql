CREATE TABLE worker_nodes (
    worker text PRIMARY KEY,
    os text NOT NULL,
    arch text NOT NULL,
    cpus integer NOT NULL CHECK (cpus >= 1),
    slots integer NOT NULL CHECK (slots >= 1),
    capabilities jsonb NOT NULL,
    first_seen timestamptz NOT NULL DEFAULT now(),
    last_seen timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX live_executions_by_worker ON executions(worker) WHERE result IS NULL AND abandoned_at IS NULL;
