CREATE TABLE project_decisions (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    request_key text NOT NULL,
    statement text NOT NULL,
    source text NOT NULL,
    validity text NOT NULL,
    valid_until timestamptz,
    evidence_refs text[] NOT NULL CHECK (cardinality(evidence_refs)>0),
    supersedes bigint CHECK (supersedes < id),
    UNIQUE (project_id,request_key),
    UNIQUE (project_id,id),
    FOREIGN KEY (project_id,supersedes) REFERENCES project_decisions(project_id,id)
);

CREATE TABLE project_decision_events (
    id bigserial PRIMARY KEY,
    decision_id bigint NOT NULL REFERENCES project_decisions(id),
    status text NOT NULL CHECK (status IN ('PROPOSED','CONFIRMED','REJECTED','SUPERSEDED','EXPIRED')),
    actor text NOT NULL,
    at timestamptz NOT NULL DEFAULT statement_timestamp(),
    UNIQUE(decision_id,status)
);

CREATE VIEW project_decision_index AS
SELECT d.*, CASE WHEN e.status IN ('PROPOSED','CONFIRMED') AND d.valid_until<=statement_timestamp()
                 THEN 'EXPIRED' ELSE e.status END AS status,
       (SELECT actor FROM project_decision_events WHERE decision_id=d.id AND status IN ('CONFIRMED','REJECTED')) AS decided_by,
       (SELECT jsonb_agg(jsonb_build_object('status',v.status,'actor',v.actor,'at',v.at) ORDER BY v.id)
        FROM project_decision_events v WHERE v.decision_id=d.id) AS history
FROM project_decisions d
JOIN LATERAL (SELECT status FROM project_decision_events WHERE decision_id=d.id ORDER BY id DESC LIMIT 1) e ON true;
