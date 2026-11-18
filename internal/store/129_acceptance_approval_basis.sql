-- Past decisions did not retain the project policy in effect at approval.
ALTER TABLE acceptance_reviews ADD COLUMN approval_basis jsonb;
