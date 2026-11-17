CREATE TABLE global_rule_revisions (
    revision integer PRIMARY KEY CHECK (revision>0),
    content text NOT NULL CHECK (octet_length(content)<=16384),
    content_sha256 text NOT NULL,
    published_by text NOT NULL DEFAULT 'Owner',
    published_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
