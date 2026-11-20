package store

import (
	"context"
	"database/sql"
	"time"
)

type ProjectActivity struct {
	ID          int64     `json:"id"`
	TaskID      int64     `json:"task_id"`
	ExecutionID int64     `json:"execution_id,omitempty"`
	Title       string    `json:"title"`
	Event       string    `json:"event"`
	At          time.Time `json:"at"`
	PullRequest string    `json:"pull_request,omitempty"`
}

func (s *Store) RecentProjectActivity(ctx context.Context, project int64) ([]ProjectActivity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT v.id,t.id,v.execution_id,`+taskSummaryTitle+`,v.type,v.at,d.pr_url
		FROM task_events v JOIN work_items t ON t.id=v.task_id
		LEFT JOIN deliveries d ON d.task_id=t.id AND d.execution_id=v.execution_id AND v.type IN ('pr_merged','delivery_completed')
		WHERE t.project_id=$1 AND v.type IN ('execution_failed','execution_retry_requested',
			'acceptance_approved','acceptance_rejected','pr_merged','delivery_completed')
		ORDER BY v.id DESC LIMIT 6`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ProjectActivity{}
	for rows.Next() {
		var item ProjectActivity
		var execution sql.NullInt64
		var pr sql.NullString
		if err := rows.Scan(&item.ID, &item.TaskID, &execution, &item.Title, &item.Event, &item.At, &pr); err != nil {
			return nil, err
		}
		item.ExecutionID, item.PullRequest = execution.Int64, pr.String
		items = append(items, item)
	}
	return items, rows.Err()
}
