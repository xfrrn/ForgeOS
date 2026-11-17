ALTER TABLE material_curations ADD COLUMN goal_snapshot jsonb CHECK (goal_snapshot IS NULL OR event_id IS NOT NULL);

CREATE FUNCTION capture_goal_curation(completed_event bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO material_curations(project_id,event_id,goal_snapshot)
    SELECT g.project_id,e.id,jsonb_build_object(
        'title',r.plan->>'objective','source',g.spec->'issue'->>'url',
        'goal',jsonb_build_object('plan_revision',r.revision,'children',children.refs),
        'content',format('Completed Goal #%s, plan revision %s.%s%s',g.id,r.revision,chr(10),children.content))
    FROM task_events e JOIN work_items g ON g.id=e.task_id
    JOIN plan_revisions r ON (r.task_id,r.revision)=(g.id,g.active_revision)
    CROSS JOIN LATERAL (
        SELECT jsonb_agg(jsonb_build_object('task_id',t.id,'key',t.plan_key,'pr_url',d.pr_url,
            'candidate',d."commit",'merge_sha',d.merge_sha) ORDER BY t.id) AS refs,
            string_agg(format('Item %s, PR %s: candidate %s, base %s, merge %s.%s',
                t.plan_key,d.pr_url,d."commit",x.result->>'base_commit',d.merge_sha,
                COALESCE(' Candidate base matched the confirmed merge of prerequisite '||prereq.plan_key||'.','')),
                chr(10) ORDER BY t.id) AS content
        FROM work_items t JOIN deliveries d ON d.task_id=t.id
        JOIN executions x ON x.id=d.execution_id AND x.task_id=t.id
        LEFT JOIN LATERAL (
            SELECT string_agg(p.plan_key,', ' ORDER BY p.id) AS plan_key
            FROM work_dependencies dep JOIN work_items p ON p.id=dep.prerequisite
            JOIN deliveries prior ON prior.task_id=p.id AND prior.state='DONE'
            WHERE dep.task_id=t.id AND p.goal_id=g.id AND prior.merge_sha=x.result->>'base_commit'
        ) prereq ON true
        WHERE t.goal_id=g.id AND t.status='DONE' AND d.state='DONE'
    ) children
    WHERE e.id=completed_event AND e.type='goal_delivered' AND children.refs IS NOT NULL
$$;

CREATE FUNCTION enqueue_goal_curation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM capture_goal_curation(NEW.id);
    RETURN NEW;
END;
$$;
CREATE TRIGGER completed_goal_curation AFTER INSERT ON task_events
    FOR EACH ROW WHEN (NEW.type='goal_delivered') EXECUTE FUNCTION enqueue_goal_curation();
SELECT capture_goal_curation(id) FROM task_events WHERE type='goal_delivered' ORDER BY id;

ALTER VIEW curation_sources RENAME TO material_decision_merge_sources;
CREATE VIEW curation_sources AS
SELECT s.*,NULL::jsonb AS goal FROM material_decision_merge_sources s
UNION ALL
SELECT c.id,c.project_id,NULL,e.id,'goal_delivered',c.goal_snapshot->>'title',c.goal_snapshot->>'source',
       c.goal_snapshot->>'content',encode(sha256(convert_to(c.goal_snapshot->>'content','UTF8')),'hex'),
       e.at,NULL,format('/projects/%s/tasks/%s/events/%s/curation',c.project_id,e.task_id,e.id),
       true,NULL,c.goal_snapshot->'goal'
FROM material_curations c JOIN task_events e ON e.id=c.event_id WHERE c.goal_snapshot IS NOT NULL;
