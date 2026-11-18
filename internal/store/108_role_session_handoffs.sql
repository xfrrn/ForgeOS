ALTER TABLE management_processes
    ADD COLUMN resumed_from bigint NOT NULL DEFAULT 0,
    ADD COLUMN missing_session boolean NOT NULL DEFAULT false;
