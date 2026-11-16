CREATE TABLE repository_analyses (
    task_id bigint PRIMARY KEY REFERENCES repository_inventories(task_id),
    source jsonb NOT NULL,
    attempt integer NOT NULL DEFAULT 0,
    lease uuid,
    lease_until timestamptz,
    result jsonb,
    failure text,
    finished_at timestamptz,
    CHECK (result IS NULL OR failure IS NULL)
);
CREATE UNIQUE INDEX repository_analysis_session ON repository_analyses((result->>'session_id')) WHERE result IS NOT NULL;
