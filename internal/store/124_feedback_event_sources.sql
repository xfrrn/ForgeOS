ALTER TABLE task_events ADD COLUMN feedback_id bigint REFERENCES preview_feedback(id),
    ADD COLUMN feedback_artifact_id bigint REFERENCES feedback_artifacts(id);

-- Earlier writers committed the evidence and event together, both with transaction timestamps.
WITH facts AS (
    SELECT f.id AS feedback, NULL::bigint AS artifact, p.execution_id, 'preview_feedback_received' AS type, f.created_at AS at
    FROM preview_feedback f JOIN static_previews p ON p.id=f.preview_id
    UNION ALL
    SELECT f.id, NULL, p.execution_id, 'preview_feedback_classified', f.classified_at
    FROM preview_feedback f JOIN static_previews p ON p.id=f.preview_id WHERE f.classified_at IS NOT NULL
    UNION ALL
    SELECT f.id, a.id, p.execution_id, CASE a.mime WHEN 'image/png' THEN 'feedback_screenshot_saved' ELSE 'feedback_recording_saved' END, a.created_at
    FROM feedback_artifacts a JOIN preview_feedback f ON f.id=a.feedback_id JOIN static_previews p ON p.id=f.preview_id
    WHERE a.mime IN ('image/png','video/webm')
), matches AS (
    SELECT v.id, f.feedback, f.artifact, count(*) OVER (PARTITION BY v.id) AS facts_per_event,
           count(*) OVER (PARTITION BY f.type,f.feedback,f.artifact) AS events_per_fact
    FROM facts f JOIN executions e ON e.id=f.execution_id
    JOIN task_events v ON (v.task_id,v.execution_id,v.type,v.at)=(e.task_id,e.id,f.type,f.at)
)
UPDATE task_events v SET feedback_id=m.feedback,feedback_artifact_id=m.artifact FROM matches m
WHERE v.id=m.id AND m.facts_per_event=1 AND m.events_per_fact=1;
