package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type RequirementRevision struct {
	ID         int64     `json:"id"`
	TaskID     int64     `json:"task_id"`
	Revision   int       `json:"revision"`
	Issue      Issue     `json:"issue"`
	ObservedAt time.Time `json:"observed_at"`
}

// The caller holds the imported task lock. Polling a renamed label or a newer branch tip is not a requirement edit.
func recordRequirement(ctx context.Context, tx *sql.Tx, taskID int64, issue Issue) error {
	var revision int
	var previous []byte
	var seen sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT r.revision,r.issue,t.last_issue_update FROM requirement_revisions r JOIN work_items t ON t.id=r.task_id WHERE r.task_id=$1 ORDER BY r.revision DESC LIMIT 1`, taskID).Scan(&revision, &previous, &seen)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var old Issue
	if previous != nil {
		if err = json.Unmarshal(previous, &old); err != nil {
			return err
		}
		if old.ID != issue.ID || old.RepositoryID != issue.RepositoryID || old.Number != issue.Number {
			return ErrConflict
		}
	}
	last := seen.Time
	if !seen.Valid {
		last, _ = time.Parse(time.RFC3339, old.UpdatedAt)
	}
	stamp, parseErr := time.Parse(time.RFC3339, issue.UpdatedAt)
	if stamp.Before(last) {
		return nil
	}
	if parseErr == nil && stamp.After(last) {
		if _, err = tx.ExecContext(ctx, `UPDATE work_items SET last_issue_update=$2 WHERE id=$1`, taskID, stamp); err != nil {
			return err
		}
	}
	if previous != nil && old.Title == issue.Title && old.Body == issue.Body {
		return nil
	}
	doc, err := json.Marshal(issue)
	if err != nil {
		return err
	}
	var rid int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO requirement_revisions(task_id,revision,issue) VALUES($1,$2,$3) RETURNING id`, taskID, revision+1, string(doc)).Scan(&rid); err != nil {
		return err
	}
	if revision == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE work_items SET requirement_id=$2 WHERE id=$1`, taskID, rid)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type) VALUES ($1,'requirement_observed')`, taskID)
	}
	return err
}

func (s *Store) ObserveIssue(ctx context.Context, projectID int64, issue Issue) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM work_items t JOIN projects p ON p.id=t.project_id WHERE t.project_id=$1 AND t.github_issue_id=$2 AND p.github_repository_id=$3 FOR UPDATE OF t`, projectID, issue.ID, issue.RepositoryID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = recordRequirement(ctx, tx, id, issue); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RequirementRevision(ctx context.Context, taskID int64, revision int64) (RequirementRevision, error) {
	var r RequirementRevision
	var doc []byte
	err := s.db.QueryRowContext(ctx, `SELECT r.id,r.task_id,r.revision,r.issue,r.observed_at FROM work_items t
		JOIN requirement_revisions r ON r.task_id=COALESCE(t.goal_id,t.id) WHERE t.id=$1 AND r.revision=$2`, taskID, revision).Scan(&r.ID, &r.TaskID, &r.Revision, &doc, &r.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(doc, &r.Issue)
	return r, err
}
