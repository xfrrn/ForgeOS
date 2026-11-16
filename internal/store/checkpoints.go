package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Checkpoint struct {
	ExecutionID  int64     `json:"execution_id"`
	Sequence     int       `json:"sequence"`
	SourceTaskID int64     `json:"source_task_id"`
	After        int64     `json:"after_event"`
	Through      int64     `json:"through_event"`
	At           time.Time `json:"at"`
	SessionID    string    `json:"session_id,omitempty"`
}
type CheckpointSummary struct {
	Checkpoint
	CommentCount int `json:"comment_count"`
}
type CommentInput struct {
	EventID   int64  `json:"event_id"`
	CommentID int64  `json:"comment_id"`
	URL       string `json:"source"`
	Body      string `json:"body"`
}
type CheckpointPage struct {
	Checkpoint
	Comments []CommentInput `json:"comments"`
	More     bool           `json:"more"`
}

func (s *Store) ReadCheckpoint(ctx context.Context, id int64, lease string, seq int, after int64, session string) (CheckpointPage, error) {
	var page CheckpointPage
	if seq < 0 || after < 0 || len(session) > 128 || strings.Trim(session, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return page, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return page, err
	}
	defer tx.Rollback()
	var digest []byte
	var source int64
	err = tx.QueryRowContext(ctx, `SELECT e.lease_hash,COALESCE(t.goal_id,t.id) FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.uses_checkpoints AND t.kind='codex_issue' AND t.status='RUNNING' AND e.result IS NULL AND e.abandoned_at IS NULL AND e.lease_until>now() FOR UPDATE OF e`, id).Scan(&digest, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return page, ErrConflict
	}
	if err != nil {
		return page, err
	}
	hash := sha256.Sum256([]byte(lease))
	if subtle.ConstantTimeCompare(hash[:], digest) != 1 {
		return page, ErrConflict
	}
	if session != "" {
		// A native turn may be reported repeatedly after a dropped HTTP response.
		var stored int
		err = tx.QueryRowContext(ctx, `UPDATE execution_checkpoints SET session_id=$3 WHERE execution_id=$1 AND sequence=$2 AND (session_id IS NULL OR session_id=$3) RETURNING sequence`, id, seq, session).Scan(&stored)
		if errors.Is(err, sql.ErrNoRows) {
			return page, ErrConflict
		}
		if err != nil {
			return page, err
		}
		return page, tx.Commit()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO execution_checkpoints(execution_id,sequence,source_task_id,after_event,through_event)
		SELECT $1,$2,$3,COALESCE(p.through_event,0),COALESCE((SELECT max(id) FROM issue_comment_events WHERE task_id=$3),0)
		FROM (SELECT 1) seed LEFT JOIN execution_checkpoints p ON p.execution_id=$1 AND p.sequence=$2::integer-1
		WHERE $2=0 OR p.session_id IS NOT NULL ON CONFLICT DO NOTHING`, id, seq, source)
	if err != nil {
		return page, err
	}
	err = tx.QueryRowContext(ctx, `SELECT execution_id,sequence,source_task_id,after_event,through_event,at,COALESCE(session_id,'') FROM execution_checkpoints WHERE execution_id=$1 AND sequence=$2`, id, seq).Scan(
		&page.ExecutionID, &page.Sequence, &page.SourceTaskID, &page.After, &page.Through, &page.At, &page.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return page, ErrConflict
	}
	if err != nil {
		return page, err
	}
	if after > page.Through {
		return page, ErrConflict
	}
	if page.After == page.Through {
		page.Comments = []CommentInput{}
		return page, tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,comment_id,payload->>'html_url',payload->>'body' FROM checkpoint_comments
		WHERE execution_id=$1 AND sequence=$2 AND id>$3 ORDER BY id LIMIT 9`, id, seq, after)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	page.Comments = []CommentInput{}
	size := 0
	for rows.Next() {
		var c CommentInput
		if err = rows.Scan(&c.EventID, &c.CommentID, &c.URL, &c.Body); err != nil {
			return page, err
		}
		raw, err := json.Marshal(c)
		if err != nil {
			return page, err
		}
		size += len(raw)
		if len(page.Comments) > 0 && (size > 1<<20 || len(page.Comments) == 8) {
			page.More = true
			break
		}
		page.Comments = append(page.Comments, c)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if err = rows.Close(); err != nil {
		return page, err
	}
	return page, tx.Commit()
}

func (s *Store) Checkpoints(ctx context.Context, taskID, executionID int64) ([]CheckpointSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.execution_id,c.sequence,c.source_task_id,c.after_event,c.through_event,c.at,COALESCE(c.session_id,''),
		(SELECT count(*) FROM checkpoint_comments i WHERE i.execution_id=c.execution_id AND i.sequence=c.sequence)
		FROM execution_checkpoints c JOIN executions e ON e.id=c.execution_id WHERE e.task_id=$1 AND e.id=$2 ORDER BY c.sequence`, taskID, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CheckpointSummary{}
	for rows.Next() {
		var c CheckpointSummary
		if err = rows.Scan(&c.ExecutionID, &c.Sequence, &c.SourceTaskID, &c.After, &c.Through, &c.At, &c.SessionID, &c.CommentCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
