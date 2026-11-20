ALTER TABLE worker_nodes ADD COLUMN pool_running integer CHECK(pool_running >= 0);
CREATE TABLE worker_process_pool (
    singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
    host text REFERENCES worker_nodes(worker),
    normal_limit integer NOT NULL CHECK(normal_limit > 0),
    hard_limit integer NOT NULL CHECK(hard_limit >= normal_limit)
);
INSERT INTO worker_process_pool(normal_limit,hard_limit) VALUES(1,1);
