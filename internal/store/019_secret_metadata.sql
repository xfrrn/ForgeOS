CREATE TABLE project_secret_metadata (
    project_id bigint NOT NULL REFERENCES projects(id),
    environment text NOT NULL CHECK (environment IN ('local-mock','local-integrated','ci-test','staging','production')),
    name text NOT NULL CHECK(name ~ '^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$'),
    risk text NOT NULL CHECK(risk IN ('S0','S1','S2','S3')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(project_id,environment,name)
);
