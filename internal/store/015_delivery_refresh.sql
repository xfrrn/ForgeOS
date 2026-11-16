CREATE TABLE delivery_refreshes (
    execution_id bigint PRIMARY KEY REFERENCES acceptance_reviews(execution_id),
    task_id bigint NOT NULL REFERENCES work_items(id),
    old_delivery jsonb NOT NULL,
    new_base text NOT NULL,
    candidate text NOT NULL,
    at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX task_delivery_refreshes ON delivery_refreshes(task_id,execution_id);
