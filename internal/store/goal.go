package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type GoalChild struct {
	ID     int64  `json:"id"`
	Key    string `json:"key"`
	Status string `json:"status"`
}

// ActivatePlan materializes one reviewed revision and its dependencies in a single transaction.
func (s *Store) ActivatePlan(ctx context.Context, id int64, revision int) (Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	if err := activatePlan(ctx, tx, id, revision); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}

func activatePlan(ctx context.Context, tx *sql.Tx, id int64, revision int) error {
	var projectID int64
	var status string
	var specJSON, planJSON []byte
	var latest, active int
	err := tx.QueryRowContext(ctx, `SELECT t.project_id,t.status,t.spec,COALESCE(t.active_revision,0) FROM work_items t WHERE t.id=$1 FOR UPDATE`, id).Scan(&projectID, &status, &specJSON, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT revision,plan FROM plan_revisions WHERE task_id=$1 ORDER BY revision DESC LIMIT 1`, id).Scan(&latest, &planJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if latest != revision {
		return ErrConflict
	}
	if active == revision && (status == "ACTIVE" || status == "DONE") {
		return nil
	}
	if status != "PLANNING" {
		return ErrConflict
	}
	var plan Plan
	var spec IssueSpec
	if err = json.Unmarshal(planJSON, &plan); err != nil {
		return err
	}
	if err = json.Unmarshal(specJSON, &spec); err != nil {
		return err
	}
	if err = plan.Validate(); err != nil {
		return ErrConflict
	}
	existing, err := rollingChanges(ctx, tx, id, active, plan)
	if err != nil {
		return err
	}
	for _, child := range existing {
		if !child.changed {
			continue
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM work_dependencies WHERE task_id=$1`, child.id); err != nil {
			return err
		}
		if child.removed {
			if _, err = tx.ExecContext(ctx, `WITH removed AS (UPDATE work_items SET status='SUPERSEDED' WHERE id=$1 RETURNING id)
				INSERT INTO task_events(task_id,type) SELECT id,'goal_item_superseded' FROM removed`, child.id); err != nil {
				return err
			}
		}
	}
	ids := make(map[string]int64)
	for _, m := range plan.Milestones {
		for _, item := range m.Items {
			prior, found := existing[item.Key]
			if found && !prior.changed {
				ids[item.Key] = prior.id
				continue
			}
			child := spec
			child.Mode = ""
			child.VerifyCommit = ""
			child.Issue.Title = item.Objective
			child.Issue.Body = fmt.Sprintf("Goal: %s\nMilestone: %s\nScope: %s\nAcceptance: %s\nDeliverables: %s", plan.Objective, m.Name, item.Scope, item.Acceptance, item.Deliverables)
			if len(item.ContextRefs) > 0 {
				child.Issue.Body += "\nReferences: " + strings.Join(item.ContextRefs, ", ")
			}
			doc, err := json.Marshal(child)
			if err != nil {
				return err
			}
			state := "READY"
			if len(item.Dependencies) > 0 {
				state = "BLOCKED"
			}
			var childID int64
			if found {
				err = tx.QueryRowContext(ctx, `WITH changed AS (UPDATE work_items SET status=$2,spec=$3,plan_revision=$4 WHERE id=$1 RETURNING id)
					INSERT INTO task_events(task_id,type) SELECT id,'goal_item_revised' FROM changed RETURNING task_id`, prior.id, state, string(doc), revision).Scan(&childID)
			} else {
				err = tx.QueryRowContext(ctx, `WITH child AS (
				INSERT INTO work_items(project_id,kind,status,spec,goal_id,plan_revision,plan_key,requirement_id)
				VALUES ($1,'codex_issue',$2,$3,$4,$5,$6,(SELECT requirement_id FROM work_items WHERE id=$4)) RETURNING id)
				INSERT INTO task_events(task_id,type) SELECT id,'goal_item_created' FROM child RETURNING task_id`,
					projectID, state, string(doc), id, revision, item.Key).Scan(&childID)
			}
			if err != nil {
				return err
			}
			ids[item.Key] = childID
		}
	}
	for _, m := range plan.Milestones {
		for _, item := range m.Items {
			for _, dep := range item.Dependencies {
				if _, err = tx.ExecContext(ctx, `INSERT INTO work_dependencies(task_id,prerequisite) VALUES ($1,$2) ON CONFLICT DO NOTHING`, ids[item.Key], ids[dep]); err != nil {
					return err
				}
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='ACTIVE',active_revision=$2 WHERE id=$1`, id, revision); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type) VALUES ($1,'plan_activated')`, id); err != nil {
		return err
	}
	return nil
}
