ALTER TABLE preview_feedback
    ADD COLUMN review_lease uuid,
    ADD COLUMN review_until timestamptz,
    ADD COLUMN classification jsonb,
    ADD COLUMN classified_at timestamptz;
CREATE INDEX feedback_waiting_management ON preview_feedback(id) WHERE classified_at IS NULL;
CREATE UNIQUE INDEX feedback_management_session ON preview_feedback((classification->>'session_id')) WHERE classified_at IS NOT NULL;
