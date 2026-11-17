ALTER TABLE management_processes
    ADD COLUMN reviewer_execution bigint REFERENCES code_reviews(execution_id),
    DROP CONSTRAINT management_process_target,
    ADD CONSTRAINT management_process_target CHECK (
        (task_id IS NULL)=(revision IS NULL)
        AND num_nonnulls(task_id,share_id,analysis_task,question_id,feedback_id,curation_id,test_plan_id,reviewer_execution)=1),
    ADD UNIQUE(reviewer_execution,lease_hash);
