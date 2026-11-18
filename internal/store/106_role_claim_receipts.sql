CREATE TABLE role_claim_receipts (
 project_id bigint NOT NULL REFERENCES projects(id), role text NOT NULL,
 node text NOT NULL, request_key text NOT NULL, request_digest bytea NOT NULL,
 response bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(project_id,role,node,request_key)
);
