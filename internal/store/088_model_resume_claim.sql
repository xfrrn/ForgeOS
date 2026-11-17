ALTER TABLE model_pauses ADD COLUMN claim_required boolean NOT NULL DEFAULT false,
    ADD COLUMN resume_claim bytea CHECK (resume_claim IS NULL OR (claim_required AND resumed_at IS NOT NULL AND octet_length(resume_claim)=32));
