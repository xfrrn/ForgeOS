ALTER TABLE management_processes
    ADD COLUMN test_plan_id bigint,
    ADD COLUMN test_plan_role text,
    ADD FOREIGN KEY(test_plan_id,test_plan_role) REFERENCES candidate_test_plan_jobs(plan_id,role),
    DROP CONSTRAINT management_process_target,
    ADD CONSTRAINT management_process_target CHECK (
        (task_id IS NULL)=(revision IS NULL)
        AND num_nonnulls(task_id,share_id,analysis_task,question_id,feedback_id,curation_id,test_plan_id)=1),
    ADD CHECK (
        (test_plan_id IS NULL AND test_plan_role IS NULL)
        OR (test_plan_id IS NOT NULL AND test_plan_role IS NOT NULL AND test_plan_role IN ('test','management'))),
    ADD UNIQUE(test_plan_id,test_plan_role,lease_hash);
