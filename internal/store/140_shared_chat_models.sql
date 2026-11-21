CREATE TABLE chat_models (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    base_url text NOT NULL DEFAULT '',
    output_format text NOT NULL DEFAULT '',
    revision uuid,
    legacy_project bigint UNIQUE,
    CHECK (revision IS NOT NULL OR legacy_project IS NOT NULL)
);
-- Existing credentials are in OpenBao. Resolve these rows once when the catalog opens.
INSERT INTO chat_models (legacy_project) SELECT id FROM projects;
ALTER TABLE project_chat_turns ADD COLUMN model_profile_id uuid, ADD COLUMN model_revision uuid,
    ADD COLUMN model_profile_name text NOT NULL DEFAULT '';
