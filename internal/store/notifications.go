package store

import (
	"context"
	"time"
)

type Notification struct {
	ID           int64      `json:"id"`
	TaskID       int64      `json:"task_id"`
	ProjectID    int64      `json:"project_id"`
	ExecutionID  *int64     `json:"execution_id,omitempty"`
	PlanRevision *int       `json:"plan_revision,omitempty"`
	Title        string     `json:"title"`
	Kind         string     `json:"kind"`
	Status       string     `json:"status"`
	At           time.Time  `json:"at"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	Pending      bool       `json:"pending"`
}

type NotificationPage struct {
	Items      []Notification `json:"items"`
	NextBefore int64          `json:"next_before,omitempty"`
}

func (s *Store) Notifications(ctx context.Context, before int64, history bool) (NotificationPage, error) {
	page := NotificationPage{Items: []Notification{}}
	rows, err := s.db.QueryContext(ctx, `SELECT id,task_id,project_id,execution_id,plan_revision,title,kind,status,at,read_at,pending
		FROM owner_notifications WHERE ($1::bigint=0 OR id<$1) AND ($2 OR pending) ORDER BY id DESC LIMIT 31`, before, history)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var n Notification
		if err = rows.Scan(&n.ID, &n.TaskID, &n.ProjectID, &n.ExecutionID, &n.PlanRevision, &n.Title, &n.Kind, &n.Status, &n.At, &n.ReadAt, &n.Pending); err != nil {
			return page, err
		}
		page.Items = append(page.Items, n)
	}
	if len(page.Items) > 30 {
		page.Items = page.Items[:30]
		page.NextBefore = page.Items[29].ID
	}
	return page, rows.Err()
}

func (s *Store) UnreadNotifications(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM owner_notifications WHERE pending AND read_at IS NULL").Scan(&n)
	return n, err
}

func (s *Store) ReadNotification(ctx context.Context, id int64) error {
	var exists bool
	err := s.db.QueryRowContext(ctx, `WITH notice AS (SELECT id FROM owner_notifications WHERE id=$1),
		ack AS (INSERT INTO owner_notification_reads(event_id) SELECT id FROM notice ON CONFLICT DO NOTHING)
		SELECT EXISTS(SELECT 1 FROM notice)`, id).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
