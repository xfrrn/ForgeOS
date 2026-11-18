ALTER TABLE task_events ADD COLUMN dependency_base text;

-- The child may have changed again after release. Use its last preceding plan event.
UPDATE task_events v SET plan_revision=(
    SELECT p.plan_revision FROM task_events p
    WHERE p.task_id=v.task_id AND p.id<v.id AND p.type IN ('goal_item_created','goal_item_revised')
    ORDER BY p.id DESC LIMIT 1
) WHERE v.type='dependencies_delivered' AND v.plan_revision IS NULL;
