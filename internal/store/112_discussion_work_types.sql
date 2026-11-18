ALTER TABLE discussion_inbox ADD COLUMN work_type text CHECK (
    work_type IN ('development','code-review','testing','investigation','operations','clarification')
);
ALTER TABLE discussion_inbox ADD COLUMN source_digest text CHECK (source_digest ~ '^[a-f0-9]{64}$');
