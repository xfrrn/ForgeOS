CREATE TABLE issue_outcome_feedback (
    event_id bigint PRIMARY KEY REFERENCES task_events(id),
    retry_at timestamptz NOT NULL DEFAULT now(),
    lease uuid,
    confirmed_at timestamptz
);
CREATE INDEX issue_outcome_due ON issue_outcome_feedback(retry_at,event_id) WHERE confirmed_at IS NULL;

CREATE FUNCTION queue_issue_outcome() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO issue_outcome_feedback(event_id)
    SELECT NEW.id FROM work_items WHERE id=NEW.task_id AND kind='codex_issue'
    AND status IN ('FAILED','CANCELLED');
    RETURN NEW;
END $$;
CREATE TRIGGER queue_issue_outcome AFTER INSERT ON task_events FOR EACH ROW
WHEN (NEW.type IN ('execution_failed','execution_abandoned','owner_cancelled','source_issue_cancelled'))
EXECUTE FUNCTION queue_issue_outcome();
