CREATE TABLE checkpoint_resumes (
    question_id bigint PRIMARY KEY REFERENCES decision_questions(id),
    claimant_hash bytea CHECK (octet_length(claimant_hash)=32),
    claimed_at timestamptz,
    CHECK ((claimant_hash IS NULL) = (claimed_at IS NULL))
);
