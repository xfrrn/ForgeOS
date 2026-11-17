ALTER TABLE model_budgets
    ADD COLUMN agent_roles jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(agent_roles)='array'),
    DROP CONSTRAINT model_budgets_check,
    ADD CHECK (cardinality(workers) + cardinality(reviewer_projects) + jsonb_array_length(test_plan_roles) + jsonb_array_length(agent_roles) > 0);

ALTER TABLE material_curations ADD COLUMN model_budget text;
CREATE INDEX leased_curation_budget ON material_curations(model_budget)
    WHERE model_budget IS NOT NULL AND review IS NULL;

CREATE OR REPLACE VIEW model_budget_usage AS
    SELECT b.name,b.harness,b.slots,b.workers,
        (SELECT count(*) FROM executions e WHERE e.model_budget=b.name AND e.result IS NULL AND e.abandoned_at IS NULL)
        + (SELECT count(*) FROM code_reviews r WHERE r.model_budget=b.name AND r.report IS NULL
            AND r.failed_at IS NULL AND r.lease_until>clock_timestamp())
        + (SELECT count(*) FROM candidate_test_plan_jobs j WHERE j.model_budget=b.name AND j.completed_at IS NULL
            AND j.lease_until>clock_timestamp())
        + (SELECT count(*) FROM material_curations c WHERE c.model_budget=b.name AND c.review IS NULL AND
            CASE WHEN c.extraction IS NULL THEN c.extraction_until ELSE c.review_until END >clock_timestamp()) AS reserved,
        b.reviewer_projects,b.test_plan_roles,b.agent_roles
    FROM model_budgets b;
