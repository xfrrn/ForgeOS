ALTER TABLE model_budgets
    ADD COLUMN test_plan_roles jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(test_plan_roles)='array'),
    DROP CONSTRAINT model_budgets_check,
    ADD CHECK (cardinality(workers) + cardinality(reviewer_projects) + jsonb_array_length(test_plan_roles) > 0);

ALTER TABLE candidate_test_plan_jobs ADD COLUMN model_budget text;
CREATE INDEX leased_test_plan_budget ON candidate_test_plan_jobs(model_budget,lease_until)
    WHERE model_budget IS NOT NULL AND completed_at IS NULL;

CREATE OR REPLACE VIEW model_budget_usage AS
    SELECT b.name,b.harness,b.slots,b.workers,
        (SELECT count(*) FROM executions e WHERE e.model_budget=b.name AND e.result IS NULL AND e.abandoned_at IS NULL)
        + (SELECT count(*) FROM code_reviews r WHERE r.model_budget=b.name AND r.report IS NULL
            AND r.failed_at IS NULL AND r.lease_until>clock_timestamp())
        + (SELECT count(*) FROM candidate_test_plan_jobs j WHERE j.model_budget=b.name AND j.completed_at IS NULL
            AND j.lease_until>clock_timestamp()) AS reserved,
        b.reviewer_projects,b.test_plan_roles
    FROM model_budgets b;
