package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type FeedbackArtifact struct {
	ID     int64  `json:"id"`
	Key    string `json:"-"`
	MIME   string `json:"mime"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func (s *Store) SaveFeedbackScreenshot(ctx context.Context, task int64, preview string, feedback int64, digest string, size int64, write func(string) error) (FeedbackArtifact, error) {
	var a FeedbackArtifact
	if len(digest) != 64 || !validCommit(digest) || size <= 0 {
		return a, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return a, err
	}
	defer tx.Rollback()
	var execution int64
	err = tx.QueryRowContext(ctx, `SELECT p.execution_id FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
		WHERE p.task_id=$1 AND p.id=$2 AND f.id=$3 FOR UPDATE OF f`, task, preview, feedback).Scan(&execution)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a = FeedbackArtifact{Key: fmt.Sprintf("tasks/%d/feedback/%d/%s.png", task, feedback, digest), MIME: "image/png", Bytes: size, SHA256: digest}
	err = tx.QueryRowContext(ctx, `INSERT INTO feedback_artifacts(feedback_id,storage_key,mime,bytes,sha256) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(feedback_id,sha256) DO NOTHING RETURNING id`, feedback, a.Key, a.MIME, size, digest).Scan(&a.ID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT id FROM feedback_artifacts WHERE feedback_id=$1 AND sha256=$2`, feedback, digest).Scan(&a.ID)
	} else if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES($1,$2,'feedback_screenshot_saved')`, task, execution)
	}
	if err != nil {
		return a, err
	}
	// Publish the complete file before committing its receipt. A retry can restore a missing file.
	if err = write(a.Key); err != nil {
		return a, err
	}
	return a, tx.Commit()
}

func (s *Store) FeedbackArtifact(ctx context.Context, task int64, preview string, feedback, id int64) (FeedbackArtifact, error) {
	var a FeedbackArtifact
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.storage_key,a.mime,a.bytes,a.sha256 FROM feedback_artifacts a
		JOIN preview_feedback f ON f.id=a.feedback_id JOIN static_preview_receipts p ON p.id=f.preview_id
		WHERE p.task_id=$1 AND p.id=$2 AND f.id=$3 AND a.id=$4`, task, preview, feedback, id).Scan(&a.ID, &a.Key, &a.MIME, &a.Bytes, &a.SHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}
