ALTER TABLE plan_deliverables
    ADD COLUMN start_date date,
    ADD COLUMN end_date date,
    ADD CHECK (start_date <= end_date);

-- Keep the existing view columns in place for clients and append the schedule.
CREATE OR REPLACE VIEW planning_delivery_summary AS
SELECT d.id, d.project_id, d.milestone, d.title, d.body, d.accepted, d.improvement,
       coalesce((SELECT jsonb_agg((to_jsonb(t)-'project_id') || jsonb_build_object(
           'removable', NOT EXISTS(SELECT 1 FROM project_chat_actions a WHERE a.plan_deliverable=d.id AND a.task_id=t.id))
           ORDER BY t.id)
           FROM planning_linked_tasks l JOIN planning_tasks t ON t.id=l.task AND t.project_id=l.project_id
           WHERE l.deliverable=d.id),'[]') AS tasks,
       d.start_date, d.end_date
FROM plan_deliverables d;
