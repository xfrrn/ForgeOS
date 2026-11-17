CREATE TABLE material_embeddings (
    project_id bigint NOT NULL,
    key text NOT NULL,
    revision integer NOT NULL,
    model text NOT NULL,
    start_char integer NOT NULL CHECK (start_char >= 0),
    end_char integer NOT NULL CHECK (end_char > start_char),
    embedding real[] NOT NULL CHECK (array_ndims(embedding)=1 AND cardinality(embedding)>0),
    PRIMARY KEY (project_id,key,revision,model,start_char),
    FOREIGN KEY (project_id,key,revision) REFERENCES project_materials(project_id,key,revision)
);
