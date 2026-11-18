ALTER TABLE discussion_inbox ADD COLUMN classification_requested boolean NOT NULL DEFAULT false;
CREATE TABLE issue_classifications (
    id bigserial PRIMARY KEY,
    project_id bigint NOT NULL REFERENCES projects(id),
    issue_id bigint NOT NULL,
    source_digest text NOT NULL,
    source jsonb NOT NULL,
    lease uuid,
    lease_until timestamptz,
    model_budget text,
    suggestion jsonb,
    UNIQUE(project_id,issue_id,source_digest)
);
ALTER TABLE requirement_revisions ADD COLUMN issue_classification_id bigint REFERENCES issue_classifications(id);
ALTER TABLE management_processes
    ADD COLUMN issue_classification_id bigint REFERENCES issue_classifications(id),
    DROP CONSTRAINT management_process_target,
    ADD CONSTRAINT management_process_target CHECK (
        (task_id IS NULL)=(revision IS NULL)
        AND num_nonnulls(task_id,share_id,analysis_task,question_id,feedback_id,curation_id,test_plan_id,reviewer_execution,issue_classification_id)=1),
    ADD UNIQUE(issue_classification_id,lease_hash,instance);

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
            AND c.lease_until>clock_timestamp()) AS reserved,
        b.reviewer_projects,b.test_plan_roles,b.agent_roles
    FROM model_budgets b;
