ALTER TABLE project_chat_turns DROP CONSTRAINT project_chat_turns_proposal_role_check,
    ADD CHECK (proposal_role IN ('lead','advisor','comparison','followup','design_draft','design_review','design_final','design_visual')),
    ADD COLUMN design_wireframe jsonb,
    ADD CHECK (design_wireframe IS NULL OR (proposal_role IN ('design_draft','design_final') AND design_version IS NOT NULL));

-- Freeze the reviewed layout when formal dispatch is registered.
ALTER TABLE project_chat_actions ADD COLUMN design_wireframe jsonb;
