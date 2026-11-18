package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrSourceClosed = errors.New("source GitHub issue closed; stop execution and report its result")

type TrackedIssue struct {
	ID, Number int64
}

// Accepted deliveries and an in-flight Goal closure reconcile their own remote writes.
const pendingSource = `t.goal_id IS NULL AND t.github_issue_id IS NOT NULL AND t.source_closure_id IS NULL
	AND t.close_lease IS NULL AND t.status IN ('READY','RUNNING','PLANNING','ACTIVE','BLOCKED','WAITING_INPUT','WAITING_ACCEPTANCE','PAUSED','FAILED')`

func (s *Store) TrackedIssues(ctx context.Context, project, repository int64) ([]TrackedIssue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.github_issue_id,(t.spec->'issue'->>'number')::bigint
		FROM work_items t JOIN projects p ON p.id=t.project_id WHERE p.id=$1 AND p.github_repository_id=$2 AND `+pendingSource+
		` ORDER BY t.id`, project, repository)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var issues []TrackedIssue
	for rows.Next() {
		var v TrackedIssue
		if err := rows.Scan(&v.ID, &v.Number); err != nil {
			return nil, err
		}
		issues = append(issues, v)
	}
	return issues, rows.Err()
}

func (s *Store) CloseSourceIssue(ctx context.Context, project, repository int64, issue TrackedIssue, url, reason string, closed, updated time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var task int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM work_items t JOIN projects p ON p.id=t.project_id
		WHERE p.id=$1 AND p.github_repository_id=$2 AND t.github_issue_id=$3 AND (t.spec->'issue'->>'number')::bigint=$4
		AND t.spec->'issue'->>'url'=$5 AND (t.last_issue_update IS NULL OR t.last_issue_update<=$6) AND `+pendingSource+` FOR UPDATE OF t`,
		project, repository, issue.ID, issue.Number, url, updated).Scan(&task)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO github_issue_closures(task_id,repository_id,issue_id,number,url,closed_at,state_reason)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, task, repository, issue.ID, issue.Number, url, closed, reason)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `WITH stopped AS (
		UPDATE work_items SET source_closure_id=$1,waiting_execution_id=NULL,status=CASE WHEN status='RUNNING' OR waiting_execution_id IS NOT NULL THEN 'RUNNING' ELSE 'CANCELLED' END
		WHERE (id=$1 OR goal_id=$1) AND status IN ('READY','RUNNING','PLANNING','ACTIVE','BLOCKED','WAITING_INPUT','WAITING_ACCEPTANCE','PAUSED','FAILED')
		RETURNING id,status
	) INSERT INTO task_events(task_id,execution_id,type,origin)
	SELECT id,(SELECT max(e.id) FROM executions e WHERE e.task_id=stopped.id),
		CASE WHEN status='RUNNING' THEN 'source_cancel_requested' ELSE 'source_issue_cancelled' END,'github' FROM stopped`, task)
	if err != nil {
		return err
	}
	return tx.Commit()
}
