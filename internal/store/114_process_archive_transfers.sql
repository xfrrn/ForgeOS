CREATE TABLE process_archive_transfers (
    execution_id bigint NOT NULL REFERENCES executions(id),
    instance bigint NOT NULL,
    received bigint NOT NULL DEFAULT 0,
    verified boolean NOT NULL DEFAULT false,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY(execution_id,instance)
);
CREATE INDEX process_archive_expiry ON process_archive_transfers(expires_at);
CREATE TABLE process_archive_chunks (
    execution_id bigint NOT NULL,
    instance bigint NOT NULL,
    start_byte bigint NOT NULL,
    content bytea NOT NULL,
    PRIMARY KEY(execution_id,instance,start_byte),
    FOREIGN KEY(execution_id,instance) REFERENCES process_archive_transfers ON DELETE CASCADE
);
