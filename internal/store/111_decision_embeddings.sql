CREATE TABLE decision_embeddings (
    decision_id bigint NOT NULL REFERENCES project_decisions(id),
    model text NOT NULL,
    embedding real[] NOT NULL CHECK (array_ndims(embedding)=1 AND cardinality(embedding)>0),
    PRIMARY KEY (decision_id,model)
);
