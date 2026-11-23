CREATE TABLE feature_proposals (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL,
    feature_id bigint NOT NULL,
    version integer NOT NULL,
    client_id uuid NOT NULL,
    request jsonb NOT NULL,
    snapshot jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    approved_revision integer,
    approved_at timestamptz,
    approved_by text,
    UNIQUE(project_id, feature_id, id),
    UNIQUE(feature_id, version),
    UNIQUE(feature_id, client_id),
    FOREIGN KEY(project_id,feature_id) REFERENCES feature_topics(project_id,id)
);
ALTER TABLE project_chat_turns ADD COLUMN proposal_id bigint,
    ADD COLUMN proposal_role text CHECK(proposal_role IN ('lead','advisor','comparison','followup')),
    ADD COLUMN proposal_parent bigint REFERENCES project_chat_turns(id),
    ADD COLUMN proposal_prompt text,
    ADD FOREIGN KEY(project_id,feature_id,proposal_id) REFERENCES feature_proposals(project_id,feature_id,id);
CREATE INDEX proposal_jobs ON project_chat_turns(proposal_id,proposal_role,id) WHERE proposal_id IS NOT NULL;
CREATE TABLE feature_proposal_revisions (
    proposal_id bigint NOT NULL REFERENCES feature_proposals(id),
    revision integer NOT NULL,
    body text NOT NULL,
    source_turn bigint NOT NULL REFERENCES project_chat_turns(id),
    author text NOT NULL,
    client_id uuid,
    UNIQUE(proposal_id,client_id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(proposal_id,revision)
);
ALTER TABLE feature_proposals ADD FOREIGN KEY(id,approved_revision) REFERENCES feature_proposal_revisions(proposal_id,revision);
