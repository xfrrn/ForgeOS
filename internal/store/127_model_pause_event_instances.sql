ALTER TABLE task_events ADD COLUMN model_pause_instance bigint,
    ADD FOREIGN KEY (execution_id,model_pause_instance) REFERENCES model_pauses(execution_id,instance);

-- These transitions were serialized on the execution, with one event per successful change.
WITH events AS (
    SELECT id,execution_id,type,row_number() OVER (PARTITION BY execution_id,type ORDER BY id) AS position,
           count(*) OVER (PARTITION BY execution_id,type) AS total
    FROM task_events WHERE type IN ('model_pause_requested','model_paused','model_resumed')
), pauses AS (
    SELECT execution_id,instance,type,row_number() OVER (PARTITION BY execution_id,type ORDER BY resource_step) AS position,
           count(*) OVER (PARTITION BY execution_id,type) AS total
    FROM model_pauses CROSS JOIN LATERAL (VALUES
        ('model_pause_requested',requested_at),('model_paused',parked_at),('model_resumed',resumed_at)
    ) AS transition(type,at) WHERE transition.at IS NOT NULL
)
UPDATE task_events v SET model_pause_instance=p.instance
FROM events e JOIN pauses p USING (execution_id,type,position,total) WHERE v.id=e.id;
