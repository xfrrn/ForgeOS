CREATE TABLE knowledge_model_settings (
    project_id bigint NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('curator','management')),
    model_id uuid REFERENCES chat_models(id) ON DELETE SET NULL,
    revision bigint NOT NULL DEFAULT 0 CHECK (revision >= 0),
    PRIMARY KEY(project_id,role)
);
CREATE INDEX knowledge_model_settings_by_model ON knowledge_model_settings(model_id) WHERE model_id IS NOT NULL;

-- Even a never-configured role has a row to lock, so its first switch to API
-- mode serializes with any last claim or result from an existing CLI worker.
INSERT INTO knowledge_model_settings(project_id,role)
    SELECT p.id,r.role FROM projects p CROSS JOIN (VALUES ('curator'),('management')) r(role);
CREATE FUNCTION initialize_knowledge_model_settings() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO knowledge_model_settings(project_id,role) VALUES(NEW.id,'curator'),(NEW.id,'management');
    RETURN NEW;
END;
$$;
CREATE TRIGGER initialize_knowledge_roles AFTER INSERT ON projects
    FOR EACH ROW EXECUTE FUNCTION initialize_knowledge_model_settings();

-- Deleting a catalog model also disables its bindings and invalidates work
-- pinned to them. A disabled role keeps a nonzero revision until rebound.
CREATE FUNCTION revise_knowledge_model_setting() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.revision := OLD.revision + 1;
    RETURN NEW;
END;
$$;
CREATE TRIGGER revise_knowledge_role BEFORE UPDATE OF model_id ON knowledge_model_settings
    FOR EACH ROW EXECUTE FUNCTION revise_knowledge_model_setting();
