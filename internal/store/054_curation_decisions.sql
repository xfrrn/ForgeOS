ALTER TABLE material_curations ADD UNIQUE (project_id,id);
ALTER TABLE project_decisions
    ADD COLUMN curation_id bigint,
    ADD COLUMN curation_fact integer,
    ADD CHECK ((curation_id IS NULL AND curation_fact IS NULL) OR
               (curation_id IS NOT NULL AND curation_fact IS NOT NULL AND curation_fact BETWEEN 1 AND 8)),
    ADD FOREIGN KEY (project_id,curation_id) REFERENCES material_curations(project_id,id);
CREATE INDEX decisions_by_curation ON project_decisions(curation_id,curation_fact,id) WHERE curation_id IS NOT NULL;
