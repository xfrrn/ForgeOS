ALTER TABLE model_budgets
    ADD COLUMN reviewer_projects bigint[] NOT NULL DEFAULT '{}',
    DROP CONSTRAINT model_budgets_workers_check,
    ADD CHECK (cardinality(workers) + cardinality(reviewer_projects) > 0);

ALTER TABLE code_reviews ADD COLUMN model_budget text;
CREATE INDEX leased_review_model_budget ON code_reviews(model_budget,lease_until)
    WHERE model_budget IS NOT NULL AND report IS NULL AND failed_at IS NULL;

CREATE OR REPLACE VIEW model_budget_usage AS
    SELECT b.name,b.harness,b.slots,b.workers,
        (SELECT count(*) FROM executions e WHERE e.model_budget=b.name AND e.result IS NULL AND e.abandoned_at IS NULL)
        + (SELECT count(*) FROM code_reviews r WHERE r.model_budget=b.name AND r.report IS NULL
            AND r.failed_at IS NULL AND r.lease_until>clock_timestamp()) AS reserved,
        b.reviewer_projects
    FROM model_budgets b;
