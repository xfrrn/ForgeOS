ALTER TABLE repository_document_changes
    ADD COLUMN part_count integer NOT NULL DEFAULT 0,
    ADD COLUMN byte_length bigint,
    ADD COLUMN file_sha256 text;
CREATE INDEX pending_document_parts ON repository_document_changes(project_id,id) WHERE problem='size_limit';
CREATE TABLE repository_document_parts (
    change_id bigint NOT NULL REFERENCES repository_document_changes(id),
    part integer NOT NULL CHECK (part>0),
    byte_offset bigint NOT NULL CHECK (byte_offset>=0),
    content text NOT NULL CHECK (octet_length(content)>0),
    PRIMARY KEY (change_id,part)
);
ALTER TABLE material_curations
    DROP CONSTRAINT material_curations_document_id_key,
    ADD COLUMN document_part integer CHECK (document_part IS NULL OR document_id IS NOT NULL),
    ADD FOREIGN KEY (document_id,document_part) REFERENCES repository_document_parts(change_id,part),
    ADD UNIQUE (document_id,document_part);
CREATE UNIQUE INDEX single_document_curation ON material_curations(document_id)
    WHERE document_id IS NOT NULL AND document_part IS NULL;

CREATE OR REPLACE VIEW curation_sources AS
SELECT s.*,NULL::jsonb AS document FROM non_document_curation_sources s
UNION ALL
SELECT c.id,d.project_id,NULL,d.id,'repository_document',d.path,
       d.commit_sha||':'||d.path||CASE WHEN p.part IS NULL THEN '' ELSE format('#bytes=%s-%s',p.byte_offset,p.byte_offset+octet_length(p.content)-1) END,
       src.content,encode(sha256(convert_to(src.content,'UTF8')),'hex'),d.observed_at,NULL,
       format('/projects/%s/document-changes/%s/curation',d.project_id,d.id)||CASE WHEN p.part IS NULL THEN '' ELSE format('?part=%s',p.part) END,
       NOT EXISTS (SELECT 1 FROM repository_document_changes newer WHERE newer.project_id=d.project_id AND newer.path=d.path AND newer.id>d.id),
       NULL,NULL,jsonb_strip_nulls(jsonb_build_object('commit',d.commit_sha,'path',d.path,'blob',d.blob,
           'previous_commit',d.previous_commit,'previous_blob',d.previous_blob,'part',p.part,'byte_offset',p.byte_offset,
           'part_count',NULLIF(d.part_count,0),'byte_length',d.byte_length,'file_sha256',d.file_sha256))
FROM repository_document_changes d JOIN material_curations c ON (c.project_id,c.document_id)=(d.project_id,d.id)
LEFT JOIN repository_document_parts p ON (p.change_id,p.part)=(c.document_id,c.document_part)
CROSS JOIN LATERAL (SELECT COALESCE(p.content,d.content) AS content) src;
