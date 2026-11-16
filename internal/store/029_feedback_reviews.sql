CREATE TABLE acceptance_review_feedback (
    feedback_id bigint PRIMARY KEY REFERENCES preview_feedback(id),
    execution_id bigint NOT NULL REFERENCES acceptance_reviews(execution_id)
);
CREATE INDEX feedback_for_review ON acceptance_review_feedback(execution_id,feedback_id);

CREATE VIEW candidate_review_feedback AS
SELECT f.id, p.task_id, p.execution_id, p.commit, p.id AS preview_id, f.body->>'note' AS note,
       CASE f.classification->>'category' WHEN 'acceptance' THEN 'approve' ELSE 'reject' END AS decision
FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
WHERE f.classified_at IS NOT NULL AND f.classification->>'category' IN ('acceptance','bug','ui_change');
