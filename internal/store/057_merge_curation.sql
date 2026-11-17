ALTER TABLE material_curations
    ADD COLUMN event_id bigint UNIQUE REFERENCES task_events(id),
    DROP CONSTRAINT material_curations_check,
    ADD CHECK (num_nonnulls(key,decision_id,event_id)=1 AND (key IS NULL)=(revision IS NULL));

CREATE VIEW merge_curation_sources AS
SELECT e.id AS event_id,t.project_id,t.id AS task_id,e.at,d.pr_number,d.pr_url,d."commit",d.merge_sha,d.target_branch,
       format('PR #%s merged into %s.%sCandidate: %s%sMerge: %s%sOwner acceptance: %s',
           d.pr_number,d.target_branch,chr(10),d."commit",chr(10),d.merge_sha,chr(10),a.note) AS content
FROM task_events e JOIN work_items t ON t.id=e.task_id
JOIN deliveries d ON (d.task_id,d.execution_id)=(e.task_id,e.execution_id)
JOIN acceptance_reviews a ON (a.task_id,a.execution_id,a."commit")=(d.task_id,d.execution_id,d."commit")
WHERE e.type='pr_merged' AND d.state IN ('MERGED','DONE') AND a.decision='approve';

CREATE FUNCTION enqueue_merge_curation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO material_curations(project_id,event_id)
        SELECT project_id,event_id FROM merge_curation_sources WHERE event_id=NEW.id;
    RETURN NEW;
END;
$$;
CREATE TRIGGER merged_pr_curation AFTER INSERT ON task_events
    FOR EACH ROW WHEN (NEW.type='pr_merged') EXECUTE FUNCTION enqueue_merge_curation();
INSERT INTO material_curations(project_id,event_id)
    SELECT project_id,event_id FROM merge_curation_sources ORDER BY event_id;

ALTER VIEW curation_sources RENAME TO material_and_decision_sources;
CREATE VIEW curation_sources AS
SELECT s.*,NULL::jsonb AS delivery FROM material_and_decision_sources s
UNION ALL
SELECT c.id,m.project_id,NULL,m.event_id,'pr_merged',format('PR #%s',m.pr_number),m.pr_url,m.content,
       encode(sha256(convert_to(m.content,'UTF8')),'hex'),m.at,NULL,
       format('/projects/%s/tasks/%s/events/%s/curation',m.project_id,m.task_id,m.event_id),true,
       jsonb_build_object('task_id',m.task_id,'pr_number',m.pr_number,'candidate',m."commit",
                          'merge_sha',m.merge_sha,'target_branch',m.target_branch)
FROM material_curations c JOIN merge_curation_sources m ON (m.project_id,m.event_id)=(c.project_id,c.event_id);
