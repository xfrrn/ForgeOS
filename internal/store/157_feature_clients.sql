CREATE TABLE feature_clients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id bigint NOT NULL REFERENCES projects(id),
    node text NOT NULL,
    instance text NOT NULL,
    backend text NOT NULL CHECK (backend IN ('claude','codex')),
    model text NOT NULL,
    seen_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(project_id,node,instance,backend,model)
);
ALTER TABLE project_chat_turns ADD COLUMN native_client uuid REFERENCES feature_clients(id);
CREATE INDEX feature_client_node ON feature_clients(project_id,node,seen_at);
