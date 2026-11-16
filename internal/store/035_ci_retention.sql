ALTER TABLE projects ADD COLUMN ci_retention jsonb NOT NULL DEFAULT '{}';

CREATE TABLE ci_archive_files (
    execution_id bigint NOT NULL REFERENCES executions(id),
    file text NOT NULL CHECK (file ~ '^[a-z][a-z0-9_-]{0,63}(\.log|-[0-9]+\.bin)$'),
    kind text NOT NULL CHECK (kind IN ('log','report','artifact')),
    confirmed_at timestamptz NOT NULL,
    expired_at timestamptz,
    removed_at timestamptz,
    PRIMARY KEY (execution_id,file)
);
CREATE INDEX ci_archive_pending_removal ON ci_archive_files(execution_id,file)
    WHERE expired_at IS NOT NULL AND removed_at IS NULL;

CREATE FUNCTION ci_declared_files(run jsonb) RETURNS TABLE(file text,kind text)
LANGUAGE sql IMMUTABLE STRICT AS $$
    WITH checks AS (SELECT value AS c FROM jsonb_array_elements(run->'checks'))
    SELECT c->>'log','log' FROM checks WHERE c->'log_digest'->>'sha256' IS NOT NULL
    UNION ALL
    SELECT o->>'file',o->>'kind' FROM checks, jsonb_array_elements(c->'outputs') o
$$;

INSERT INTO ci_archive_files(execution_id,file,kind,confirmed_at)
SELECT e.id,f.file,f.kind,v.at FROM executions e
JOIN task_events v ON v.execution_id=e.id AND v.type IN ('execution_completed','execution_failed','planner_completed')
CROSS JOIN LATERAL ci_declared_files(e.result->'validation'->'run') f;

CREATE FUNCTION record_ci_archive_files() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO ci_archive_files(execution_id,file,kind,confirmed_at)
    SELECT NEW.id,f.file,f.kind,now() FROM ci_declared_files(NEW.result->'validation'->'run') f;
    RETURN NEW;
END $$;
CREATE TRIGGER record_ci_archive_files AFTER UPDATE OF result ON executions
    FOR EACH ROW WHEN (OLD.result IS NULL AND NEW.result->'validation'->'run'->>'id' IS NOT NULL)
    EXECUTE FUNCTION record_ci_archive_files();
