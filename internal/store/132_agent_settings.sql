CREATE TABLE agent_config_revisions (
 revision bigserial PRIMARY KEY,
 scope_id bigint NOT NULL CHECK (scope_id >= 0),
 purpose text NOT NULL,
 settings jsonb
);
CREATE INDEX agent_config_scope ON agent_config_revisions(scope_id,purpose,revision DESC);

CREATE TABLE agent_assignments (
 id bigserial PRIMARY KEY,
 project_id bigint NOT NULL REFERENCES projects(id),
 ticket_hash bytea NOT NULL UNIQUE,
 node text NOT NULL,
 purpose text NOT NULL,
 config jsonb NOT NULL,
 assigned_at timestamptz NOT NULL DEFAULT now(),
 observed_at timestamptz,
 harness text,
 model text,
 session_id text,
 outcome text
);
CREATE INDEX agent_assignment_history ON agent_assignments(project_id,purpose,id DESC);
