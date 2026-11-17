ALTER TABLE decision_questions ADD COLUMN trees jsonb,
    ADD CHECK (trees IS NULL OR checkpoint_sequence IS NOT NULL);
