CREATE TABLE secret_grants (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL,
    task_id bigint NOT NULL REFERENCES work_items(id),
    execution_id bigint NOT NULL REFERENCES executions(id),
    principal text NOT NULL,
    environment text NOT NULL CHECK(environment <> 'production'),
    name text NOT NULL,
    risk text NOT NULL CHECK(risk IN ('S0','S1','S2')),
    metadata_updated_at timestamptz NOT NULL,
    version integer NOT NULL CHECK(version > 0),
    purpose text NOT NULL,
    expires_at timestamptz NOT NULL,
    max_uses integer CHECK(max_uses > 0),
    request_key text NOT NULL,
    approved_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    revoked_at timestamptz,
    UNIQUE (execution_id,request_key),
    FOREIGN KEY(project_id,environment,name) REFERENCES project_secret_metadata(project_id,environment,name)
);

CREATE TABLE secret_grant_uses (
    id bigserial PRIMARY KEY,
    grant_id bigint NOT NULL REFERENCES secret_grants(id),
    request_key text NOT NULL,
    outcome text NOT NULL CHECK(outcome IN ('ISSUED','FAILED')),
    at timestamptz NOT NULL DEFAULT clock_timestamp(),
    wrapper_expires_at timestamptz,
    UNIQUE(grant_id,request_key)
);
