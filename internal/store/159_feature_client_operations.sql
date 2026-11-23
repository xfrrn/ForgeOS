ALTER TABLE feature_clients ADD COLUMN probe_status text NOT NULL DEFAULT 'ready',
  ADD COLUMN checked_at timestamptz, ADD COLUMN resource_worker text NOT NULL DEFAULT '';
UPDATE feature_clients SET probe_status = CASE WHEN isfinite(seen_at) THEN 'ready' ELSE 'removed' END,
  checked_at = CASE WHEN isfinite(seen_at) THEN seen_at END;
CREATE TABLE feature_probe_requests (
  project_id bigint NOT NULL REFERENCES projects(id), node text NOT NULL,
  requested bigint NOT NULL DEFAULT 0, completed bigint NOT NULL DEFAULT 0,
  PRIMARY KEY(project_id,node)
);
ALTER TABLE project_chat_turns ADD COLUMN cancel_requested boolean NOT NULL DEFAULT false,
  ADD COLUMN recovery_of bigint REFERENCES project_chat_turns(id), ADD COLUMN session_missing boolean NOT NULL DEFAULT false,
  ADD COLUMN resource_worker text, ADD COLUMN model_budget text;
CREATE UNIQUE INDEX feature_turn_recovery ON project_chat_turns(recovery_of) WHERE recovery_of IS NOT NULL;
CREATE INDEX feature_running_resources ON project_chat_turns(resource_worker,model_budget) WHERE status='running';
CREATE OR REPLACE VIEW model_budget_usage AS
    SELECT b.name,b.harness,b.slots,b.workers,
        (SELECT count(*) FROM executions e WHERE e.model_budget=b.name AND e.result IS NULL AND e.abandoned_at IS NULL)
        + (SELECT count(*) FROM code_reviews r WHERE r.model_budget=b.name AND r.report IS NULL
            AND r.failed_at IS NULL AND r.lease_until>clock_timestamp())
        + (SELECT count(*) FROM candidate_test_plan_jobs j WHERE j.model_budget=b.name AND j.completed_at IS NULL
            AND j.lease_until>clock_timestamp())
        + (SELECT count(*) FROM material_curations c WHERE c.model_budget=b.name AND c.review IS NULL AND
            CASE WHEN c.extraction IS NULL THEN c.extraction_until ELSE c.review_until END >clock_timestamp())
        + (SELECT count(*) FROM plan_revisions p WHERE p.model_budget=b.name AND p.review IS NULL
            AND p.review_until>clock_timestamp())
        + (SELECT count(*) FROM knowledge_shares s WHERE s.model_budget=b.name AND s.review IS NULL
            AND s.review_until>clock_timestamp())
        + (SELECT count(*) FROM repository_analyses a WHERE a.model_budget=b.name AND a.finished_at IS NULL
            AND a.lease_until>clock_timestamp())
        + (SELECT count(*) FROM preview_feedback f WHERE f.model_budget=b.name AND f.classified_at IS NULL
            AND f.review_until>clock_timestamp())
        + (SELECT count(*) FROM question_advice_claims c JOIN decision_questions q ON q.id=c.question_id
            WHERE c.model_budget=b.name AND q.management_advice IS NULL AND c.lease_until>clock_timestamp())
        + (SELECT count(*) FROM issue_classifications c WHERE c.model_budget=b.name AND c.suggestion IS NULL
            AND c.lease_until>clock_timestamp()) + (SELECT count(*) FROM project_chat_turns t WHERE t.model_budget=b.name
            AND t.status='running' AND t.lease_until>clock_timestamp()) AS reserved,
        b.reviewer_projects,b.test_plan_roles,b.agent_roles
    FROM model_budgets b;
