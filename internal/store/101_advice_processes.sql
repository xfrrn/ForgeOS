ALTER TABLE management_processes
    ADD COLUMN question_id bigint REFERENCES decision_questions(id),
    DROP CONSTRAINT management_process_target,
    ADD CONSTRAINT management_process_target CHECK (
        (task_id IS NULL)=(revision IS NULL)
        AND num_nonnulls(task_id,share_id,analysis_task,question_id)=1),
    ADD UNIQUE(question_id,lease_hash);
