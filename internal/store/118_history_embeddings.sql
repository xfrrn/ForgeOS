CREATE TABLE history_embeddings (
    task_id bigint NOT NULL REFERENCES work_items(id),
    content_hash bytea NOT NULL CHECK (octet_length(content_hash)=32),
    model text NOT NULL,
    embedding real[] NOT NULL CHECK (array_ndims(embedding)=1 AND cardinality(embedding)>0),
    PRIMARY KEY (task_id,content_hash,model)
);
