ALTER TABLE management_processes
    ADD COLUMN analysis_task bigint REFERENCES repository_analyses(task_id),
    DROP CONSTRAINT management_process_target,
    ADD CONSTRAINT management_process_target CHECK (
        (task_id IS NULL)=(revision IS NULL)
        AND num_nonnulls(task_id,share_id,analysis_task)=1),
    ADD UNIQUE(analysis_task,lease_hash);
