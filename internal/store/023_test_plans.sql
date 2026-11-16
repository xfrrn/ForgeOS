CREATE TABLE candidate_test_plans (
    id bigserial PRIMARY KEY,
    execution_id bigint NOT NULL REFERENCES executions(id),
    task_id bigint NOT NULL REFERENCES work_items(id),
    revision integer NOT NULL CHECK (revision > 0),
    requested_criteria jsonb NOT NULL,
    source jsonb NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(execution_id,revision)
);
CREATE INDEX candidate_test_plans_by_task ON candidate_test_plans(task_id,execution_id);
CREATE TABLE candidate_test_plan_jobs (
    plan_id bigint NOT NULL REFERENCES candidate_test_plans(id),
    role text NOT NULL CHECK(role IN ('test','management')),
    lease uuid,
    lease_until timestamptz,
    outcome jsonb,
    failed boolean NOT NULL DEFAULT false,
    completed_at timestamptz,
    PRIMARY KEY(plan_id,role),
    CHECK (NOT failed OR outcome IS NULL)
);
CREATE UNIQUE INDEX unique_test_plan_agent_session ON candidate_test_plan_jobs((outcome->>'session_id')) WHERE outcome IS NOT NULL;
CREATE VIEW candidate_test_plan_receipts AS
SELECT p.id,p.execution_id,p.task_id,p.revision,p.requested_at,p.source->'basis' AS basis,
       a.outcome AS plan, a.completed_at AS planned_at, m.outcome AS coverage, m.completed_at AS reviewed_at,
       CASE WHEN a.failed THEN 'plan_failed' WHEN m.failed THEN 'review_failed' WHEN m.outcome IS NOT NULL THEN m.outcome->>'state'
            WHEN a.outcome IS NOT NULL THEN 'awaiting_review' ELSE 'awaiting_plan' END AS state
FROM candidate_test_plans p JOIN candidate_test_plan_jobs a ON a.plan_id=p.id AND a.role='test'
LEFT JOIN candidate_test_plan_jobs m ON m.plan_id=p.id AND m.role='management';
