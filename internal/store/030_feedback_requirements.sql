ALTER TABLE requirement_revisions ADD COLUMN feedback_id bigint UNIQUE REFERENCES preview_feedback(id);
