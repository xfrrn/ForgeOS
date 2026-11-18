ALTER TABLE task_events ADD COLUMN requirement_version_id bigint REFERENCES requirement_revisions(id);

-- The creation and observation events shared a transaction with their revision. Adoption did not.
WITH matches AS (
    SELECT v.id AS event_id,r.id AS revision_id,count(*) OVER (PARTITION BY v.id) AS revisions,
           count(*) OVER (PARTITION BY r.id) AS events
    FROM requirement_revisions r JOIN task_events v ON v.task_id=r.task_id AND v.at=r.observed_at
      AND v.type=CASE WHEN r.feedback_id IS NULL THEN 'requirement_observed' ELSE 'requirement_revised_from_feedback' END
    WHERE r.revision>1
)
UPDATE task_events v SET requirement_version_id=m.revision_id FROM matches m
WHERE v.id=m.event_id AND m.revisions=1 AND m.events=1;
