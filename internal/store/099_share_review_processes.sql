ALTER TABLE plan_review_processes RENAME TO management_processes;
ALTER TABLE management_processes
    ALTER COLUMN task_id DROP NOT NULL,
    ALTER COLUMN revision DROP NOT NULL,
    ADD COLUMN share_id bigint REFERENCES knowledge_shares(id),
    ADD CONSTRAINT management_process_target CHECK (
        (share_id IS NULL AND task_id IS NOT NULL AND revision IS NOT NULL)
        OR (share_id IS NOT NULL AND task_id IS NULL AND revision IS NULL)),
    ADD UNIQUE(share_id,lease_hash);
