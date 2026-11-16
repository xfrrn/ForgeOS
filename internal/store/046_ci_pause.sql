ALTER TABLE executions ADD COLUMN ci_candidate text,
    ADD COLUMN ci_pause_requested_at timestamptz,
    ADD COLUMN ci_resumed_at timestamptz,
    ADD CHECK (ci_pause_requested_at IS NULL OR ci_candidate IS NOT NULL),
    ADD CHECK (ci_resumed_at IS NULL OR ci_pause_requested_at IS NOT NULL);
ALTER TABLE work_items DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('PLANNING','ACTIVE','BLOCKED','READY','RUNNING','PAUSED','DONE','FAILED','WAITING_ACCEPTANCE','ACCEPTED','WAITING_INPUT','SUPERSEDED','CANCELLED'));
