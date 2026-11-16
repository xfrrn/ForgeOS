ALTER TABLE executions ADD COLUMN uses_checkpoints boolean NOT NULL DEFAULT false;

CREATE TABLE execution_checkpoints (
    execution_id bigint NOT NULL REFERENCES executions(id),
    sequence integer NOT NULL CHECK(sequence>=0),
    source_task_id bigint NOT NULL REFERENCES work_items(id),
    after_event bigint NOT NULL,
    through_event bigint NOT NULL CHECK(through_event>=after_event),
    at timestamptz NOT NULL DEFAULT now(),
    session_id text,
    PRIMARY KEY(execution_id,sequence)
);

CREATE VIEW checkpoint_comments AS
SELECT c.execution_id,c.sequence,e.id,e.comment_id,e.payload
FROM execution_checkpoints c CROSS JOIN LATERAL (
    SELECT DISTINCT ON (i.comment_id) i.id,i.comment_id,i.payload
    FROM issue_comment_events i
    WHERE c.through_event>c.after_event AND i.task_id=c.source_task_id AND i.id<=c.through_event
    ORDER BY i.comment_id,i.updated_at DESC,i.id DESC
) e WHERE e.id>c.after_event;
