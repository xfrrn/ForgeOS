ALTER TABLE executions ADD live_ci_log jsonb;

CREATE FUNCTION clear_live_ci_log() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.live_ci_log := NULL;
    RETURN NEW;
END
$$;
CREATE TRIGGER clear_settled_ci_log BEFORE UPDATE OF result,abandoned_at ON executions
    FOR EACH ROW WHEN (NEW.live_ci_log IS NOT NULL AND (NEW.result IS NOT NULL OR NEW.abandoned_at IS NOT NULL))
    EXECUTE FUNCTION clear_live_ci_log();
