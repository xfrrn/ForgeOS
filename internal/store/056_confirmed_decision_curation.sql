ALTER TABLE material_curations
    ALTER COLUMN key DROP NOT NULL,
    ALTER COLUMN revision DROP NOT NULL,
    ADD COLUMN decision_id bigint,
    ADD FOREIGN KEY (project_id,decision_id) REFERENCES project_decisions(project_id,id),
    ADD UNIQUE (decision_id),
    ADD CHECK ((decision_id IS NULL AND key IS NOT NULL AND revision IS NOT NULL)
            OR (decision_id IS NOT NULL AND key IS NULL AND revision IS NULL));

CREATE FUNCTION enqueue_confirmed_decision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO material_curations(project_id,decision_id)
        SELECT project_id,id FROM project_decisions WHERE id=NEW.decision_id;
    RETURN NEW;
END;
$$;
CREATE TRIGGER decision_confirmed AFTER INSERT ON project_decision_events
    FOR EACH ROW WHEN (NEW.status='CONFIRMED') EXECUTE FUNCTION enqueue_confirmed_decision();
INSERT INTO material_curations(project_id,decision_id)
    SELECT d.project_id,d.id FROM project_decisions d JOIN project_decision_events e ON e.decision_id=d.id
    WHERE e.status='CONFIRMED' ORDER BY e.id;

CREATE VIEW curation_sources AS
SELECT c.id AS curation_id,m.project_id,m.key,m.revision::bigint,m.kind,m.title,m.source,m.content,
       m.content_sha256,m.imported_at,NULL::jsonb AS decision,
       format('/projects/%s/materials/%s/revisions/%s/curation',m.project_id,m.key,m.revision) AS receipt_path,
       NOT EXISTS (SELECT 1 FROM project_materials newer WHERE newer.project_id=m.project_id
                   AND newer.key=m.key AND newer.revision>m.revision) AS is_current
FROM material_curations c JOIN project_materials m USING(project_id,key,revision)
UNION ALL
SELECT c.id,d.project_id,NULL,e.id,'confirmed_decision',format('Project decision #%s',d.id),
       format('/projects/%s/decisions/%s',d.project_id,d.id),src.content,
       encode(sha256(convert_to(src.content,'UTF8')),'hex'),e.at,
       jsonb_build_object('id',d.id,'confirmed_by',e.actor,'source',d.source,'evidence_refs',d.evidence_refs,
                          'valid_until',d.valid_until,'supersedes',d.supersedes),
       format('/projects/%s/decisions/%s/curation',d.project_id,d.id),d.status='CONFIRMED'
FROM material_curations c JOIN project_decision_index d ON (d.project_id,d.id)=(c.project_id,c.decision_id)
JOIN project_decision_events e ON e.decision_id=d.id AND e.status='CONFIRMED'
CROSS JOIN LATERAL (SELECT d.statement || E'\n\nValidity: ' || d.validity ||
    CASE WHEN d.valid_until IS NULL THEN '' ELSE E'\nValid until: ' ||
        to_char(d.valid_until AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"') END AS content) src;
