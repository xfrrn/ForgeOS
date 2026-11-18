CREATE TABLE share_embeddings (
    share_id bigint NOT NULL REFERENCES knowledge_shares(id),
    model text NOT NULL,
    embedding real[] NOT NULL CHECK (array_ndims(embedding)=1 AND cardinality(embedding)>0),
    PRIMARY KEY (share_id,model)
);
CREATE FUNCTION forget_revoked_share_embedding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM share_embeddings WHERE share_id=NEW.id;
    RETURN NEW;
END $$;
CREATE TRIGGER forget_revoked_share_embedding AFTER UPDATE OF revoked_at ON knowledge_shares
    FOR EACH ROW WHEN (OLD.revoked_at IS NULL AND NEW.revoked_at IS NOT NULL)
    EXECUTE FUNCTION forget_revoked_share_embedding();
