CREATE TABLE requirement_revisions (
    id bigserial PRIMARY KEY,
    task_id bigint NOT NULL REFERENCES work_items(id),
    revision integer NOT NULL CHECK (revision>0),
    issue jsonb NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(task_id, revision)
);

INSERT INTO requirement_revisions(task_id,revision,issue,observed_at)
SELECT t.id,1,t.spec->'issue',COALESCE((SELECT min(at) FROM task_events WHERE task_id=t.id AND type='issue_imported'),now())
FROM work_items t WHERE t.github_issue_id IS NOT NULL;

ALTER TABLE work_items ADD COLUMN requirement_id bigint REFERENCES requirement_revisions(id),
    ADD COLUMN last_issue_update timestamptz;
UPDATE work_items t SET requirement_id=r.id FROM requirement_revisions r
WHERE r.task_id=COALESCE(t.goal_id,t.id) AND r.revision=1;

ALTER TABLE executions ADD COLUMN requirement_id bigint REFERENCES requirement_revisions(id);
UPDATE executions e SET requirement_id=t.requirement_id FROM work_items t
WHERE t.id=e.task_id AND t.requirement_id IS NOT NULL;
