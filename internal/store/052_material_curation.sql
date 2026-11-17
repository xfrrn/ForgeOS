CREATE TABLE material_curations (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL,
    key text NOT NULL,
    revision integer NOT NULL,
    extraction_lease uuid,
    extraction_until timestamptz,
    extraction jsonb,
    extracted_at timestamptz,
    review_lease uuid,
    review_until timestamptz,
    review jsonb,
    reviewed_at timestamptz,
    UNIQUE (project_id,key,revision),
    FOREIGN KEY (project_id,key,revision) REFERENCES project_materials(project_id,key,revision)
);
CREATE INDEX pending_material_extractions ON material_curations(project_id,id) WHERE extraction IS NULL;
CREATE INDEX pending_material_reviews ON material_curations(project_id,id) WHERE extraction IS NOT NULL AND review IS NULL;

CREATE FUNCTION enqueue_material_curation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO material_curations(project_id,key,revision) VALUES (NEW.project_id,NEW.key,NEW.revision);
    RETURN NEW;
END;
$$;
CREATE TRIGGER material_imported AFTER INSERT ON project_materials
    FOR EACH ROW EXECUTE FUNCTION enqueue_material_curation();
INSERT INTO material_curations(project_id,key,revision)
    SELECT project_id,key,revision FROM project_materials ORDER BY imported_at,project_id,key,revision;
