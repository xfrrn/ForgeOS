ALTER TABLE agent_assignments ADD COLUMN role_lease_hash bytea;
CREATE UNIQUE INDEX agent_assignment_role_lease ON agent_assignments(role_lease_hash) WHERE role_lease_hash IS NOT NULL;

ALTER TABLE project_chat_turns ADD COLUMN requested_harness text NOT NULL DEFAULT ''
    CHECK(requested_harness IN ('','codex','claude'));
UPDATE project_chat_turns SET requested_harness='codex' WHERE requested_model<>'';
