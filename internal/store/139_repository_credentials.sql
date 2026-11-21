CREATE TABLE repository_credentials (
    id bigserial PRIMARY KEY,
    vault_key uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    name text NOT NULL,
    url text NOT NULL,
    username text NOT NULL,
    version integer NOT NULL DEFAULT 0 CHECK (version >= 0),
    enabled boolean NOT NULL DEFAULT false CHECK (NOT enabled OR version > 0),
    UNIQUE (url, name)
);
ALTER TABLE repository_clone_jobs ADD COLUMN credential_id bigint REFERENCES repository_credentials(id);
ALTER TABLE projects ADD COLUMN repository_credential_id bigint REFERENCES repository_credentials(id);
