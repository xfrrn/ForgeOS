ALTER TABLE project_chat_actions DROP CONSTRAINT project_chat_actions_kind_check;
ALTER TABLE project_chat_actions ADD CHECK (kind IN ('dispatch_issue','check_head','create_issue'));
ALTER TABLE project_chat_actions
    ADD COLUMN issue_body text NOT NULL DEFAULT '',
    ADD COLUMN create_key uuid NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN create_started_at timestamptz;
