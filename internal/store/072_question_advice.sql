ALTER TABLE decision_questions ADD COLUMN management_advice jsonb;

CREATE TABLE question_advice_claims (
    question_id bigint PRIMARY KEY REFERENCES decision_questions(id),
    lease uuid NOT NULL,
    lease_until timestamptz NOT NULL,
    sources jsonb NOT NULL CHECK (jsonb_typeof(sources)='array'),
    minimum_level text NOT NULL CHECK(minimum_level IN ('D2','D3'))
);
