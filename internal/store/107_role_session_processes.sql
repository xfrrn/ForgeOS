ALTER TABLE management_processes
    ADD COLUMN instance bigint GENERATED ALWAYS AS ((process->>'instance')::bigint) STORED;
DO $$
DECLARE constraint_name text;
BEGIN
    FOR constraint_name IN SELECT conname FROM pg_constraint
        WHERE conrelid='management_processes'::regclass AND contype='u'
    LOOP
        EXECUTE format('ALTER TABLE management_processes DROP CONSTRAINT %I', constraint_name);
    END LOOP;
END $$;
ALTER TABLE management_processes
    ADD UNIQUE(task_id,revision,lease_hash,instance),
    ADD UNIQUE(share_id,lease_hash,instance),
    ADD UNIQUE(analysis_task,lease_hash,instance),
    ADD UNIQUE(question_id,lease_hash,instance),
    ADD UNIQUE(feedback_id,lease_hash,instance),
    ADD UNIQUE(curation_id,curation_role,lease_hash,role_turn,instance),
    ADD UNIQUE(test_plan_id,test_plan_role,lease_hash,instance),
    ADD UNIQUE(reviewer_execution,lease_hash,instance);
