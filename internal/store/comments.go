package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type CommentEvent struct {
	ID         int64           `json:"id"`
	TaskID     int64           `json:"task_id"`
	ObservedAt time.Time       `json:"observed_at"`
	Payload    json.RawMessage `json:"payload"`
}

func (s *Store) RecordComments(ctx context.Context, taskID int64, issue Issue, page []json.RawMessage) error {
	doc, err := json.Marshal(page)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize appends for this source so an ID cursor cannot advance past an uncommitted earlier page.
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM work_items t JOIN projects p ON p.id=t.project_id
		WHERE t.id=$1 AND t.github_issue_id=$2 AND p.github_repository_id=$3 FOR UPDATE OF t`, taskID, issue.ID, issue.RepositoryID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO issue_comment_events(task_id,comment_id,updated_at,body_hash,payload)
		SELECT $1,(value->>'id')::bigint,(value->>'updated_at')::timestamptz,sha256(convert_to(value->>'body','UTF8')),value
		FROM jsonb_array_elements($2::jsonb) ON CONFLICT DO NOTHING`, id, string(doc))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Comments(ctx context.Context, taskID, after int64) ([]CommentEvent, error) {
	var source int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(goal_id,id) FROM work_items WHERE id=$1`, taskID).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,task_id,observed_at,payload FROM issue_comment_events WHERE task_id=$1 AND id>$2 ORDER BY id LIMIT 30`, source, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CommentEvent{}
	for rows.Next() {
		var e CommentEvent
		if err = rows.Scan(&e.ID, &e.TaskID, &e.ObservedAt, &e.Payload); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
