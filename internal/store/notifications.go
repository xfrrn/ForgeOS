package store

import (
	"context"
	"database/sql"
	"errors"
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

// Claim only pending unread notices. A crashed sender's reservation expires.
func (s *Store) ClaimExternalNotice(ctx context.Context, project int64, channel string) (Notification, int64, error) {
	var n Notification
	var attempt int64
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
 SELECT n.id,n.task_id,n.kind FROM owner_notifications n
 LEFT JOIN external_notification_deliveries d ON d.event_id=n.id AND d.channel=$2
 WHERE n.project_id=$1 AND n.pending AND n.read_at IS NULL AND d.delivered_at IS NULL
 AND (d.event_id IS NULL OR d.next_attempt<=clock_timestamp()) ORDER BY n.id LIMIT 1
), claimed AS (
 INSERT INTO external_notification_deliveries(event_id,channel,attempt,next_attempt)
 SELECT id,$2,1,clock_timestamp()+interval '1 minute' FROM candidate
 ON CONFLICT(event_id,channel) DO UPDATE SET attempt=external_notification_deliveries.attempt+1,
 next_attempt=excluded.next_attempt WHERE external_notification_deliveries.delivered_at IS NULL
 AND external_notification_deliveries.next_attempt<=clock_timestamp() RETURNING event_id,attempt
) SELECT c.id,c.task_id,c.kind,d.attempt FROM candidate c JOIN claimed d ON c.id=d.event_id`, project, channel).Scan(&n.ID, &n.TaskID, &n.Kind, &attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return Notification{}, 0, nil
	}
	n.ProjectID = project
	return n, attempt, err
}

func (s *Store) FinishExternalNotice(ctx context.Context, event int64, channel string, attempt int64, sent bool, retrySeconds int) error {
	result, err := s.db.ExecContext(ctx, `UPDATE external_notification_deliveries SET
 delivered_at=CASE WHEN $4 THEN clock_timestamp() END,
 next_attempt=clock_timestamp()+make_interval(secs=>$5)
 WHERE event_id=$1 AND channel=$2 AND attempt=$3 AND delivered_at IS NULL`, event, channel, attempt, sent, retrySeconds)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
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
