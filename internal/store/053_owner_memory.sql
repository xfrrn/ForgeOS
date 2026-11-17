ALTER TABLE project_materials
    DROP CONSTRAINT project_materials_kind_check,
    ADD CONSTRAINT project_materials_kind_check CHECK (kind IN ('profile','environment','material','memory'));
