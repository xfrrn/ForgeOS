CREATE TABLE document_embeddings (
    project_id bigint NOT NULL REFERENCES projects(id),
    blob text NOT NULL,
    model text NOT NULL,
    start_byte bigint NOT NULL,
    end_byte bigint NOT NULL CHECK (end_byte>start_byte),
    words tsvector NOT NULL,
    embedding real[] NOT NULL CHECK (array_ndims(embedding)=1 AND cardinality(embedding)>0),
    PRIMARY KEY (project_id,blob,model,start_byte)
);
