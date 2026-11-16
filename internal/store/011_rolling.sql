ALTER TABLE work_items
    ADD COLUMN active_revision integer,
    ADD FOREIGN KEY (id,active_revision) REFERENCES plan_revisions(task_id,revision),
    DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('PLANNING','ACTIVE','BLOCKED','READY','RUNNING','DONE','FAILED','WAITING_ACCEPTANCE','ACCEPTED','WAITING_INPUT','SUPERSEDED'));

UPDATE work_items g SET active_revision=(SELECT max(r.revision) FROM plan_revisions r WHERE r.task_id=g.id)
WHERE g.status IN ('ACTIVE','DONE') AND EXISTS (SELECT 1 FROM work_items c WHERE c.goal_id=g.id);
