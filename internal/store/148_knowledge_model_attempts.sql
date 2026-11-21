-- Reserve before sending private input to a model. A crash or timeout counts as
-- a turn because the provider may already have processed the request.
CREATE TABLE knowledge_model_attempts (
    project_id bigint NOT NULL REFERENCES projects(id),
    binding_revision bigint NOT NULL,
    model_revision uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('extract','curation_review','share_review')),
    job_id bigint NOT NULL,
    attempts integer NOT NULL CHECK (attempts BETWEEN 1 AND 3),
    lease uuid NOT NULL,
    retry_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    status text NOT NULL DEFAULT 'running' CHECK(status IN ('running','succeeded','failed')),
    diagnostic text NOT NULL DEFAULT '',
    PRIMARY KEY(project_id,binding_revision,model_revision,kind,job_id)
);
CREATE INDEX knowledge_model_failures_by_binding
    ON knowledge_model_attempts(project_id,binding_revision,model_revision,updated_at DESC) WHERE status='failed';
