package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"slices"
)

type existingChild struct {
	id      int64
	changed bool
	removed bool
}

type milestoneItem struct {
	milestone string
	item      PlanItem
}

func planItems(plan Plan) map[string]milestoneItem {
	items := make(map[string]milestoneItem)
	for _, m := range plan.Milestones {
		for _, item := range m.Items {
			item.Dependencies = slices.Clone(item.Dependencies)
			slices.Sort(item.Dependencies)
			if len(item.Dependencies) == 0 {
				item.Dependencies = nil
			}
			if len(item.ContextRefs) == 0 {
				item.ContextRefs = nil
			}
			items[item.Key] = milestoneItem{m.Name, item}
		}
	}
	return items
}

// Lock running and unstarted children before deciding which specifications can be replaced.
func rollingChanges(ctx context.Context, tx *sql.Tx, id int64, active int, next Plan) (map[string]existingChild, error) {
	children := make(map[string]existingChild)
	if active == 0 {
		return children, nil
	}
	var doc []byte
	if err := tx.QueryRowContext(ctx, `SELECT plan FROM plan_revisions WHERE task_id=$1 AND revision=$2`, id, active).Scan(&doc); err != nil {
		return nil, err
	}
	var previous Plan
	if err := json.Unmarshal(doc, &previous); err != nil {
		return nil, err
	}
	before, after := planItems(previous), planItems(next)
	rows, err := tx.QueryContext(ctx, `SELECT c.id,c.plan_key,c.status,EXISTS(SELECT 1 FROM executions e WHERE e.task_id=c.id)
		FROM work_items c WHERE c.goal_id=$1 ORDER BY c.id FOR UPDATE OF c`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var child existingChild
		var key, status string
		var attempted bool
		if err = rows.Scan(&child.id, &key, &status, &attempted); err != nil {
			return nil, err
		}
		item, present := after[key]
		child.removed = !present
		previouslyPresent := status != "SUPERSEDED"
		child.changed = present != previouslyPresent || (present && !reflect.DeepEqual(before[key], item))
		if child.changed && (attempted || (status != "READY" && status != "BLOCKED" && status != "SUPERSEDED")) {
			return nil, ErrConflict
		}
		if present && !attempted && previous.Objective != next.Objective {
			child.changed = true
		}
		children[key] = child
	}
	return children, rows.Err()
}
