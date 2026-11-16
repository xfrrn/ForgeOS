CREATE TABLE candidate_test_case_results (
    plan_id bigint NOT NULL REFERENCES candidate_test_plans(id),
    case_id text NOT NULL,
    attempt integer NOT NULL CHECK(attempt > 0),
    tester text NOT NULL CHECK(tester IN ('runner','human')),
    actor text NOT NULL,
    body jsonb NOT NULL CHECK(body->>'result' IN ('passed','failed','blocked')),
    recorded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(plan_id,case_id,attempt)
);

CREATE VIEW candidate_test_plan_status AS
SELECT r.*, counts.attempt_count,
       CASE WHEN r.state <> 'covered' THEN 'pending'
            WHEN counts.has_failure THEN 'failed'
            WHEN counts.has_block THEN 'blocked'
            WHEN counts.passed > 0 AND counts.passed = jsonb_array_length(r.plan->'cases') THEN 'passed'
            ELSE 'pending' END AS execution_state
FROM candidate_test_plan_receipts r
CROSS JOIN LATERAL (
    SELECT count(*) FILTER (WHERE result='passed') AS passed,
           bool_or(result='failed') AS has_failure, bool_or(result='blocked') AS has_block,
           COALESCE(sum(attempt),0)::bigint AS attempt_count
    FROM (SELECT DISTINCT ON (case_id) attempt,body->>'result' AS result FROM candidate_test_case_results
          WHERE plan_id=r.id ORDER BY case_id,attempt DESC) latest
) counts;
