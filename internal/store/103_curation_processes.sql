ALTER TABLE management_processes
    ADD COLUMN curation_id bigint REFERENCES material_curations(id),
    ADD COLUMN curation_role text,
    ADD COLUMN role_turn smallint,
    DROP CONSTRAINT management_process_target,
    ADD CONSTRAINT management_process_target CHECK (
        (task_id IS NULL)=(revision IS NULL)
        AND num_nonnulls(task_id,share_id,analysis_task,question_id,feedback_id,curation_id)=1),
    ADD CHECK (
        (curation_id IS NULL AND curation_role IS NULL AND role_turn IS NULL)
        OR (curation_id IS NOT NULL AND curation_role IS NOT NULL AND role_turn IS NOT NULL
            AND curation_role IN ('curator','management') AND role_turn IN (1,2)
            AND (curation_role='curator' OR role_turn=1))),
    ADD UNIQUE(curation_id,curation_role,lease_hash,role_turn);
