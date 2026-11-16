ALTER TABLE projects ADD COLUMN requires_code_review boolean NOT NULL DEFAULT false;
CREATE TABLE code_reviews (
    execution_id bigint PRIMARY KEY REFERENCES executions(id),
    task_id bigint NOT NULL REFERENCES work_items(id),
    base_commit text NOT NULL,
    commit_sha text NOT NULL,
    requirement text NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    lease uuid,
    lease_until timestamptz,
    report jsonb,
    completed_at timestamptz,
    failed_at timestamptz,
    owner_note text,
    owner_checked_at timestamptz
);
CREATE UNIQUE INDEX distinct_code_review_session ON code_reviews((report->>'session_id')) WHERE report IS NOT NULL;
CREATE INDEX code_reviews_by_task ON code_reviews(task_id,execution_id);
CREATE VIEW code_review_receipts AS
SELECT execution_id,task_id,base_commit,commit_sha,requested_at,report,completed_at,failed_at,owner_note,owner_checked_at
FROM code_reviews;
