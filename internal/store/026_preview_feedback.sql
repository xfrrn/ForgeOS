CREATE TABLE preview_feedback (
    id bigserial PRIMARY KEY,
    preview_id text NOT NULL REFERENCES static_previews(id),
    request_id uuid NOT NULL,
    plan_id bigint REFERENCES candidate_test_plans(id),
    body jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(preview_id,request_id)
);
CREATE INDEX feedback_by_preview ON preview_feedback(preview_id,id);
