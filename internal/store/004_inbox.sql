CREATE TABLE discussion_inbox (
    project_id bigint NOT NULL REFERENCES projects(id),
    issue_id bigint NOT NULL,
    number bigint NOT NULL,
    title text NOT NULL,
    url text NOT NULL,
    PRIMARY KEY (project_id, issue_id),
    UNIQUE (project_id, number)
);
