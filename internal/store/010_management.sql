ALTER TABLE plan_revisions
    ADD COLUMN minimum_level text CHECK (minimum_level IN ('D1','D2','D3')),
    ADD COLUMN review_lease uuid,
    ADD COLUMN review_until timestamptz,
    ADD COLUMN review jsonb,
    ADD COLUMN reviewed_at timestamptz;
