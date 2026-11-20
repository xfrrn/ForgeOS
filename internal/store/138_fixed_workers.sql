ALTER TABLE worker_nodes
    ADD COLUMN fixed_limits jsonb,
    ADD COLUMN resident_online integer CHECK(resident_online >= 0);
