CREATE TABLE knowledge_shares (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL,
    material_key text NOT NULL,
    revision integer NOT NULL,
    recipient bigint NOT NULL REFERENCES projects(id),
    title text NOT NULL,
    content text NOT NULL,
    digest text NOT NULL,
    proposed_at timestamptz NOT NULL DEFAULT now(),
    review_lease uuid,
    review_until timestamptz,
    review jsonb,
    reviewed_at timestamptz,
    published_at timestamptz,
    revoked_at timestamptz,
    FOREIGN KEY (project_id,material_key,revision) REFERENCES project_materials(project_id,key,revision),
    CHECK (project_id<>recipient),
    UNIQUE (project_id,material_key,revision,recipient,digest)
);
CREATE INDEX published_knowledge_by_recipient ON knowledge_shares(recipient,id DESC)
    WHERE published_at IS NOT NULL AND revoked_at IS NULL;
CREATE VIEW knowledge_share_receipts AS
SELECT id,project_id,material_key,revision,recipient,title,content,digest,proposed_at,
       review,reviewed_at,published_at,revoked_at
FROM knowledge_shares;
