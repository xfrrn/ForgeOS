ALTER TABLE executions ADD COLUMN decision_context jsonb CHECK (jsonb_typeof(decision_context)='array');
