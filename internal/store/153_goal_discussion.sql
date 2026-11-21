CREATE TABLE goal_discussion_turns (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id bigint NOT NULL,
    revision integer NOT NULL,
    FOREIGN KEY (task_id,revision) REFERENCES plan_revisions(task_id,revision),
    client_id uuid NOT NULL,
    UNIQUE (task_id, client_id),
    item_key text NOT NULL,
    message text NOT NULL,
    draft boolean NOT NULL,
    model_id uuid NOT NULL,
    model_revision uuid NOT NULL,
    model_name text NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','done','failed')),
    lease uuid,
    lease_until timestamptz,
    answer text NOT NULL DEFAULT '',
    generated_revision integer,
    FOREIGN KEY (task_id,generated_revision) REFERENCES plan_revisions(task_id,revision),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX goal_discussion_pending ON goal_discussion_turns(task_id) WHERE status IN ('queued','running');
CREATE INDEX goal_discussion_history ON goal_discussion_turns(task_id,id);
ALTER TABLE plan_revisions ADD COLUMN owner_activation_required boolean NOT NULL DEFAULT false;
