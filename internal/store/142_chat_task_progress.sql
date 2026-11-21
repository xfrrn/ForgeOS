ALTER TABLE project_chat_actions ADD COLUMN confirmed_at timestamptz;
-- Existing receipts have no confirmation timestamp. Preserve their turn ordering.
UPDATE project_chat_actions a SET confirmed_at=COALESCE(a.create_started_at,t.created_at)
FROM project_chat_turns t WHERE a.turn_id=t.id AND a.status='confirmed';
CREATE INDEX chat_dispatched_tasks ON project_chat_actions(project_id,confirmed_at DESC,id DESC)
    WHERE status='confirmed' AND kind IN ('create_issue','dispatch_issue');

ALTER TABLE project_chat_turns ADD COLUMN progress_task_ids bigint[] NOT NULL DEFAULT '{}';
