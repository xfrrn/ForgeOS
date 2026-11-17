ALTER TABLE candidate_test_plan_jobs ADD COLUMN profile jsonb
    CHECK (profile IS NULL OR (NOT failed AND outcome IS NOT NULL AND jsonb_typeof(profile)='object'));
