package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

var taskStatuses = []string{"PLANNING", "ACTIVE", "BLOCKED", "READY", "RUNNING", "WAITING_INPUT", "WAITING_ACCEPTANCE", "ACCEPTED", "DONE", "FAILED", "SUPERSEDED", "CANCELLED"}

func ValidTaskStatus(status string) bool {
	return slices.Contains(taskStatuses, status)
}

type TaskListFilter struct {
	Limit  int
	Before int64
	Status string
	Query  string
}

type TaskSummary struct {
	ID          int64      `json:"id"`
	ProjectID   int64      `json:"project_id"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Title       string     `json:"title"`
	Repository  string     `json:"repository"`
	IssueNumber int64      `json:"issue_number,omitempty"`
	Worker      string     `json:"worker,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at"`
	PRNumber    int64      `json:"pr_number,omitempty"`
}

type TaskPage struct {
	Items      []TaskSummary    `json:"items"`
	NextBefore int64            `json:"next_before,omitempty"`
	Counts     map[string]int64 `json:"counts"`
}

func (s *Store) ListTasks(ctx context.Context, f TaskListFilter) (TaskPage, error) {
	if f.Limit == 0 {
		f.Limit = 30
	}
	if f.Limit < 1 || f.Limit > 100 {
		return TaskPage{}, errors.New("limit must be between 1 and 100")
	}
	var page TaskPage
	var items, counts []byte
	// Page and totals share a snapshot even when a Worker changes state during a refresh.
	err := s.db.QueryRowContext(ctx, `
		WITH tasks AS (
		    SELECT t.id, t.project_id, t.kind, t.status,
		           COALESCE(NULLIF(t.spec->'issue'->>'title',''), CASE WHEN t.kind='environment_check' THEN '本地环境启动检查' ELSE 'Git HEAD 检查' END) AS title,
		           COALESCE(t.spec->'issue'->>'repository', '') AS repository,
		           t.spec->'issue'->'number' AS issue_number
		    FROM work_items t
		), page AS (
		    SELECT t.*, d.pr_number,
		           (SELECT e.worker FROM executions e WHERE e.task_id=t.id ORDER BY e.id DESC LIMIT 1) AS worker,
		           (SELECT v.at FROM task_events v WHERE v.task_id=t.id ORDER BY v.id DESC LIMIT 1) AS updated_at
		    FROM tasks t LEFT JOIN deliveries d ON d.task_id=t.id
		    WHERE ($1::bigint=0 OR t.id<$1) AND ($2::text='' OR t.status=$2)
		      AND ($3::text='' OR lower($3) IN (t.id::text, '#' || t.id::text, 't-' || t.id::text)
		           OR strpos(lower(t.title),lower($3))>0 OR strpos(lower(t.repository),lower($3))>0)
		    ORDER BY t.id DESC LIMIT $4
		), totals AS (
		    SELECT status, count(*) AS n FROM work_items GROUP BY status
		)
		SELECT COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id DESC) FROM page p), '[]'),
		       COALESCE((SELECT jsonb_object_agg(status,n) FROM totals), '{}')`,
		f.Before, f.Status, f.Query, f.Limit+1).Scan(&items, &counts)
	if err != nil {
		return TaskPage{}, err
	}
	if err := json.Unmarshal(items, &page.Items); err != nil {
		return TaskPage{}, err
	}
	page.Counts = make(map[string]int64, len(taskStatuses)+1)
	for _, status := range taskStatuses {
		page.Counts[status] = 0
	}
	if err := json.Unmarshal(counts, &page.Counts); err != nil {
		return TaskPage{}, err
	}
	var total int64
	for _, n := range page.Counts {
		total += n
	}
	page.Counts["ALL"] = total
	if len(page.Items) > f.Limit {
		page.Items = page.Items[:f.Limit]
		page.NextBefore = page.Items[f.Limit-1].ID
	}
	return page, nil
}
