ALTER TABLE plan_milestones
    ADD COLUMN start_date date,
    ADD COLUMN end_date date,
    ADD CHECK (start_date <= end_date);
