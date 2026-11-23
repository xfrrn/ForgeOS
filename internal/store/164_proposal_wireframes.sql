ALTER TABLE feature_proposal_revisions ADD COLUMN wireframe jsonb;
ALTER TABLE project_chat_turns ADD COLUMN visual_revision integer,
    ADD FOREIGN KEY (proposal_id,visual_revision) REFERENCES feature_proposal_revisions(proposal_id,revision),
    ADD CHECK (visual_revision IS NULL OR proposal_role='design_visual');
