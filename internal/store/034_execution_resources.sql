ALTER TABLE worker_nodes
    ADD COLUMN model_slots integer CHECK (model_slots > 0),
    ADD COLUMN ci_slots integer CHECK (ci_slots > 0);

ALTER TABLE executions
    ADD COLUMN resource_managed boolean NOT NULL DEFAULT false,
    ADD COLUMN resource_step bigint NOT NULL DEFAULT 0 CHECK (resource_step >= 0),
    ADD COLUMN resource_kind text NOT NULL DEFAULT '' CHECK (resource_kind IN ('', 'model', 'ci'));
