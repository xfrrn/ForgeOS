CREATE TABLE role_archive_transfers (
    process_id bigint PRIMARY KEY REFERENCES management_processes(id),
    project_id bigint NOT NULL REFERENCES projects(id),
    role text NOT NULL CHECK(role IN ('management','curator','test-agent','reviewer')),
    instance bigint NOT NULL,
    received bigint NOT NULL DEFAULT 0,
    verified boolean NOT NULL DEFAULT false,
    expires_at timestamptz NOT NULL,
    UNIQUE(process_id,instance)
);
CREATE INDEX role_archive_expiry ON role_archive_transfers(expires_at);
CREATE TABLE role_archive_chunks (
    process_id bigint NOT NULL,
    instance bigint NOT NULL,
    start_byte bigint NOT NULL,
    content bytea NOT NULL,
    PRIMARY KEY(process_id,instance,start_byte),
    FOREIGN KEY(process_id,instance) REFERENCES role_archive_transfers(process_id,instance) ON DELETE CASCADE
);
