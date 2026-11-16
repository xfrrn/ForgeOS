CREATE TABLE feedback_artifacts (
    id bigserial PRIMARY KEY,
    feedback_id bigint NOT NULL REFERENCES preview_feedback(id),
    storage_key text NOT NULL UNIQUE,
    mime text NOT NULL,
    bytes bigint NOT NULL CHECK (bytes > 0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(feedback_id,sha256)
);
