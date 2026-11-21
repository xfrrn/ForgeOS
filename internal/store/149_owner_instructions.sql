ALTER TABLE project_materials DROP CONSTRAINT project_materials_kind_check,
    ADD CHECK (kind IN ('profile','environment','material','memory','instruction'));

DROP TRIGGER material_imported ON project_materials;
CREATE TRIGGER material_imported AFTER INSERT ON project_materials
    FOR EACH ROW WHEN (NEW.kind <> 'instruction') EXECUTE FUNCTION enqueue_material_curation();

-- Keep historical receipts, while excluding repository-derived facts from all
-- effective projections used by the library, keyword search and embeddings.
CREATE OR REPLACE VIEW curation_sources AS
SELECT curation_id,project_id,key,revision,kind,title,source,content,content_sha256,imported_at,decision,receipt_path,
       is_current AND kind<>'repository_document' AS is_current,delivery,goal,document
FROM original_curation_sources
UNION ALL
SELECT c.id,d.project_id,NULL::text,d.id,'memory',d.title,d.source,body.text,
       encode(sha256(convert_to(body.text,'UTF8')),'hex'),d.created_at,NULL::jsonb,
       format('/projects/%s/knowledge/experience/%s/changes/%s/curation',d.project_id,d.entry_id,d.id),
       d.discarded_at IS NULL,NULL::jsonb,NULL::jsonb,NULL::jsonb
FROM experience_changes d JOIN material_curations c ON c.experience_change_id=d.id
CROSS JOIN LATERAL (SELECT d.content || CASE WHEN d.validity='' THEN '' ELSE E'\n\n适用场景：' || d.validity END AS text) body;
