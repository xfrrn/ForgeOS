CREATE TABLE model_budgets (
    name text PRIMARY KEY,
    harness text NOT NULL CHECK (harness IN ('codex','claude')),
    slots integer NOT NULL CHECK (slots > 0),
    workers text[] NOT NULL CHECK (cardinality(workers) > 0)
);

ALTER TABLE executions ADD COLUMN model_budget text;
CREATE INDEX active_model_budget ON executions(model_budget)
    WHERE model_budget IS NOT NULL AND result IS NULL AND abandoned_at IS NULL;

CREATE VIEW model_budget_usage AS
    SELECT b.*, (SELECT count(*) FROM executions e WHERE e.model_budget=b.name
        AND e.result IS NULL AND e.abandoned_at IS NULL) AS reserved
    FROM model_budgets b;
