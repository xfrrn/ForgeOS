CREATE VIEW current_project_experience AS
SELECT c.id,c.project_id,c.key,c.decision_id,c.event_id,s.revision,s.title,s.source,s.kind,s.receipt_path,
       s.content_sha256,s.content,c.extraction->'facts' AS facts,c.reviewed_at
FROM material_curations c JOIN curation_sources s ON s.curation_id=c.id
WHERE s.is_current AND c.review->>'verdict'='approve' AND c.review->>'level'='D1';

CREATE TABLE experience_embeddings (
    curation_id bigint NOT NULL REFERENCES material_curations(id),
    fact_number integer NOT NULL CHECK (fact_number>0),
    model text NOT NULL,
    embedding real[] NOT NULL CHECK (array_ndims(embedding)=1 AND cardinality(embedding)>0),
    PRIMARY KEY (curation_id,fact_number,model)
);
