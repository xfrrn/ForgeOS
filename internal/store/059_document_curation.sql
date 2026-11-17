CREATE TABLE repository_document_heads (
    project_id bigint PRIMARY KEY REFERENCES projects(id),
    commit_sha text NOT NULL DEFAULT '',
    files jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE repository_document_changes (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    previous_commit text NOT NULL,
    commit_sha text NOT NULL,
    path text NOT NULL,
    previous_blob text NOT NULL,
    blob text NOT NULL,
    content text,
    problem text NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((content IS NOT NULL) = (problem=''))
);
CREATE INDEX document_path_history ON repository_document_changes(project_id,path,id DESC);
CREATE INDEX document_project_history ON repository_document_changes(project_id,id DESC);
ALTER TABLE material_curations
    ADD COLUMN document_id bigint UNIQUE REFERENCES repository_document_changes(id),
    DROP CONSTRAINT material_curations_check,
    ADD CHECK (num_nonnulls(key,decision_id,event_id,document_id)=1 AND (key IS NULL)=(revision IS NULL));

CREATE FUNCTION enqueue_document_curation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO material_curations(project_id,document_id) VALUES (NEW.project_id,NEW.id);
    RETURN NEW;
END;
$$;
CREATE TRIGGER changed_document_curation AFTER INSERT ON repository_document_changes
    FOR EACH ROW WHEN (NEW.content IS NOT NULL) EXECUTE FUNCTION enqueue_document_curation();

ALTER VIEW curation_sources RENAME TO non_document_curation_sources;
CREATE VIEW curation_sources AS
SELECT s.*,NULL::jsonb AS document FROM non_document_curation_sources s
UNION ALL
SELECT c.id,d.project_id,NULL,d.id,'repository_document',d.path,d.commit_sha||':'||d.path,d.content,
       encode(sha256(convert_to(d.content,'UTF8')),'hex'),d.observed_at,NULL,
       format('/projects/%s/document-changes/%s/curation',d.project_id,d.id),
       NOT EXISTS (SELECT 1 FROM repository_document_changes newer WHERE newer.project_id=d.project_id AND newer.path=d.path AND newer.id>d.id),
       NULL,NULL,jsonb_build_object('commit',d.commit_sha,'path',d.path,'blob',d.blob,'previous_commit',d.previous_commit,'previous_blob',d.previous_blob)
FROM repository_document_changes d JOIN material_curations c ON (c.project_id,c.document_id)=(d.project_id,d.id);
