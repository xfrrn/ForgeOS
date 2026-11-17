ALTER TABLE management_processes
    ADD COLUMN feedback_id bigint REFERENCES preview_feedback(id),
    DROP CONSTRAINT management_process_target,
    ADD CONSTRAINT management_process_target CHECK (
        (task_id IS NULL)=(revision IS NULL)
        AND num_nonnulls(task_id,share_id,analysis_task,question_id,feedback_id)=1),
    ADD UNIQUE(feedback_id,lease_hash);
