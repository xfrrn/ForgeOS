ALTER TABLE native_command_approvals ADD COLUMN patch jsonb;
ALTER TABLE native_command_approvals DROP CONSTRAINT native_command_approvals_native_status_check;
ALTER TABLE native_command_approvals ADD CHECK (native_status IN ('completed','declined','failed','turn_interrupted'));
CREATE OR REPLACE VIEW live_command_approvals AS
SELECT c.* FROM native_command_approvals c
JOIN open_native_input_turns a USING(execution_id,instance,session_id,turn_id)
WHERE c.valid_until>clock_timestamp();
