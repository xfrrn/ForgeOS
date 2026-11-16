ALTER TABLE work_items DROP CONSTRAINT work_items_status_check,
    ADD CHECK (status IN ('READY', 'RUNNING', 'DONE', 'FAILED', 'WAITING_ACCEPTANCE', 'ACCEPTED'));

CREATE TABLE acceptance_reviews (
    execution_id bigint PRIMARY KEY REFERENCES executions(id),
    task_id bigint NOT NULL REFERENCES work_items(id),
    "commit" text NOT NULL,
    decision text NOT NULL CHECK (decision IN ('approve','reject')),
    note text NOT NULL,
    actor text NOT NULL DEFAULT 'owner',
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reviews_by_task ON acceptance_reviews(task_id, execution_id);

CREATE TABLE deliveries (
    task_id bigint PRIMARY KEY REFERENCES work_items(id),
    execution_id bigint NOT NULL UNIQUE REFERENCES acceptance_reviews(execution_id),
    "commit" text NOT NULL,
    branch text NOT NULL,
    target_branch text NOT NULL,
    state text NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','WAITING','MERGED','DONE')),
    detail text NOT NULL DEFAULT '',
    pr_number bigint NOT NULL DEFAULT 0 CHECK (pr_number >= 0),
    pr_url text NOT NULL DEFAULT '',
    merge_sha text NOT NULL DEFAULT '',
    lease_token uuid,
    available_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((pr_number = 0) = (pr_url = '')),
    CHECK (pr_number = 0 OR target_branch <> ''),
    CHECK ((state IN ('MERGED','DONE')) = (merge_sha <> '')),
    CHECK (merge_sha = '' OR pr_number > 0)
);
CREATE INDEX due_deliveries ON deliveries(available_at, task_id) WHERE state <> 'DONE';
