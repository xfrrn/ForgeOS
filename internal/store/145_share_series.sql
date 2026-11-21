CREATE TABLE knowledge_share_series (
    id bigint PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    recipient bigint NOT NULL REFERENCES projects(id),
    current_share_id bigint,
    publication_epoch bigint NOT NULL DEFAULT 0,
    CHECK (project_id <> recipient),
    UNIQUE (project_id, recipient, id)
);

-- Old publications were independent. Keep every historical receipt and grant.
INSERT INTO knowledge_share_series(id,project_id,recipient)
    SELECT id,project_id,recipient FROM knowledge_shares;
ALTER TABLE knowledge_shares
    ALTER COLUMN material_key DROP NOT NULL,
    ALTER COLUMN revision DROP NOT NULL,
    ADD COLUMN experience_id bigint,
    ADD COLUMN experience_revision integer,
    ADD COLUMN series_id bigint,
    ADD COLUMN share_version integer NOT NULL DEFAULT 1 CHECK (share_version > 0),
    ADD COLUMN parent_share_id bigint,
    ADD COLUMN base_share_id bigint,
    ADD COLUMN base_epoch bigint NOT NULL DEFAULT 0,
    ADD COLUMN superseded_at timestamptz;
UPDATE knowledge_shares SET series_id=id;
-- Replace the legacy content deduplication constraint with one for each
-- initial source type. Later drafts are deduplicated within their own series.
DO $$
DECLARE legacy_name name;
BEGIN
    SELECT conname INTO STRICT legacy_name FROM pg_constraint
    WHERE conrelid='knowledge_shares'::regclass AND contype='u'
      AND pg_get_constraintdef(oid)='UNIQUE (project_id, material_key, revision, recipient, digest)';
    EXECUTE format('ALTER TABLE knowledge_shares DROP CONSTRAINT %I', legacy_name);
END
$$;
ALTER TABLE knowledge_shares
    ALTER COLUMN series_id SET NOT NULL,
    ADD CHECK ((material_key IS NOT NULL AND revision IS NOT NULL AND experience_id IS NULL AND experience_revision IS NULL)
            OR (experience_id IS NOT NULL AND experience_revision IS NOT NULL AND material_key IS NULL AND revision IS NULL)),
    ADD FOREIGN KEY (project_id,experience_id,experience_revision)
        REFERENCES experience_revisions(project_id,entry_id,revision),
    ADD FOREIGN KEY (project_id,recipient,series_id)
        REFERENCES knowledge_share_series(project_id,recipient,id),
    ADD UNIQUE (series_id,id),
    ADD UNIQUE (series_id,share_version),
    ADD FOREIGN KEY (series_id,parent_share_id) REFERENCES knowledge_shares(series_id,id),
    ADD FOREIGN KEY (series_id,base_share_id) REFERENCES knowledge_shares(series_id,id);
ALTER TABLE knowledge_share_series ADD FOREIGN KEY (id,current_share_id) REFERENCES knowledge_shares(series_id,id);
UPDATE knowledge_share_series g SET current_share_id=k.id,publication_epoch=1
FROM knowledge_shares k WHERE g.id=k.series_id AND k.published_at IS NOT NULL AND k.revoked_at IS NULL
    AND k.review->>'verdict'='approve';
CREATE UNIQUE INDEX knowledge_share_initial_material
    ON knowledge_shares(project_id,material_key,revision,recipient,digest) WHERE share_version=1 AND material_key IS NOT NULL;
CREATE UNIQUE INDEX knowledge_share_initial_experience
    ON knowledge_shares(project_id,experience_id,experience_revision,recipient,digest) WHERE share_version=1 AND experience_id IS NOT NULL;
CREATE UNIQUE INDEX knowledge_share_version_retry
    ON knowledge_shares(series_id,parent_share_id,base_epoch,coalesce(revision,experience_revision),digest)
    WHERE parent_share_id IS NOT NULL;

CREATE OR REPLACE VIEW knowledge_share_receipts AS
SELECT k.id,k.project_id,k.material_key,k.revision,k.recipient,k.title,k.content,k.digest,k.proposed_at,
       k.review,k.reviewed_at,k.published_at,k.revoked_at,k.series_id,k.share_version,g.current_share_id,
       k.parent_share_id,k.base_share_id,k.superseded_at,k.experience_id,k.experience_revision,g.publication_epoch,
       (k.published_at IS NULL AND k.revoked_at IS NULL AND k.base_epoch<>g.publication_epoch) AS stale
FROM knowledge_shares k JOIN knowledge_share_series g ON g.id=k.series_id;

CREATE VIEW visible_knowledge_shares AS
SELECT k.id,k.series_id,k.share_version,g.recipient,k.title,k.content,k.digest,k.published_at
FROM knowledge_share_series g JOIN knowledge_shares k ON k.id=g.current_share_id AND k.series_id=g.id
    AND k.recipient=g.recipient
WHERE k.published_at IS NOT NULL AND k.revoked_at IS NULL AND k.superseded_at IS NULL
    AND k.review->>'verdict'='approve';
CREATE INDEX current_shared_by_recipient ON knowledge_share_series(recipient,current_share_id DESC)
    WHERE current_share_id IS NOT NULL;
CREATE INDEX outgoing_shares_by_project ON knowledge_shares(project_id,id DESC);
CREATE TRIGGER forget_superseded_share_embedding AFTER UPDATE OF superseded_at ON knowledge_shares
    FOR EACH ROW WHEN (NEW.superseded_at IS NOT NULL AND OLD.superseded_at IS NULL)
    EXECUTE FUNCTION forget_revoked_share_embedding();

-- Owner and independent share reviewers resolve the private source through this view.
CREATE VIEW knowledge_share_sources AS
SELECT project_id,key AS material_key,revision,NULL::bigint AS experience_id,NULL::integer AS experience_revision,
       title,source,content,true AS is_effective
FROM project_materials
UNION ALL
SELECT project_id,NULL,NULL,entry_id,revision,title,receipt_path,concat_ws(E'\n',statement,nullif(validity,''),quote),is_effective
FROM experience_revision_facts;
