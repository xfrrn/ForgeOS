ALTER TABLE project_chat_turns DROP CONSTRAINT project_chat_turns_proposal_role_check,
    ADD COLUMN design_version integer,
    ADD CHECK (proposal_role IN ('lead','advisor','comparison','followup','design_draft','design_review','design_final')),
    ADD CHECK (design_version IS NULL OR (design_version>0 AND proposal_role IN ('design_draft','design_review','design_final')));
CREATE INDEX feature_design_history ON project_chat_turns(proposal_id,design_version,id) WHERE design_version IS NOT NULL;

CREATE INDEX feature_dispatch_task ON project_chat_actions(project_id,task_id) WHERE task_id IS NOT NULL;
