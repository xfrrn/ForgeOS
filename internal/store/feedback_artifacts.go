package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrScreenshotExpired = errors.New("反馈截图已按项目保留策略到期")

func (s *Store) ScreenshotRetention(ctx context.Context, project int64, seconds *int32) (int32, error) {
	var current int32
	if seconds == nil {
		err := s.db.QueryRowContext(ctx, "SELECT screenshot_seconds FROM projects WHERE id=$1", project).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return current, err
	}
	if *seconds < 0 {
		return 0, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var before, after []byte
	err = tx.QueryRowContext(ctx, `SELECT jsonb_build_object('seconds',screenshot_seconds),jsonb_build_object('seconds',$2::integer)
		FROM projects WHERE id=$1 FOR UPDATE`, project, *seconds).Scan(&before, &after)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	revision, err := recordRetentionChange(ctx, tx, project, "feedback_screenshot", before, after)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE projects SET screenshot_seconds=$2,screenshot_revision=COALESCE($3,screenshot_revision) WHERE id=$1", project, *seconds, revision); err != nil {
		return 0, err
	}
	// Only live receipts adopt the new deadline. An expired receipt stays expired.
	_, err = tx.ExecContext(ctx, `UPDATE feedback_artifacts a SET expires_at=a.created_at + make_interval(secs=>NULLIF($2::int,0)),
		retention_revision=project.screenshot_revision
		FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
		JOIN projects project ON project.id=p.project_id
		WHERE a.feedback_id=f.id AND p.project_id=$1 AND a.removed_at IS NULL AND (a.expires_at IS NULL OR a.expires_at>statement_timestamp())`, project, *seconds)
	if err != nil {
		return 0, err
	}
	return *seconds, tx.Commit()
}

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
	var seconds int32
	var revision sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT p.execution_id,project.screenshot_seconds,project.screenshot_revision FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
		JOIN projects project ON project.id=p.project_id
		WHERE p.task_id=$1 AND p.id=$2 AND f.id=$3 FOR SHARE OF project FOR UPDATE OF f`, task, preview, feedback).Scan(&execution, &seconds, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a = FeedbackArtifact{Key: fmt.Sprintf("tasks/%d/feedback/%d/%s.png", task, feedback, digest), MIME: "image/png", Bytes: size, SHA256: digest}
	err = tx.QueryRowContext(ctx, `INSERT INTO feedback_artifacts(feedback_id,storage_key,mime,bytes,sha256,expires_at,retention_revision)
		VALUES($1,$2,$3,$4,$5,now()+make_interval(secs=>NULLIF($6::int,0)),$7)
		ON CONFLICT(feedback_id,sha256) DO NOTHING RETURNING id`, feedback, a.Key, a.MIME, size, digest, seconds, revision).Scan(&a.ID)
	if errors.Is(err, sql.ErrNoRows) {
		var expired bool
		err = tx.QueryRowContext(ctx, `SELECT id,removed_at IS NOT NULL OR COALESCE(expires_at<=clock_timestamp(),false),retention_revision
			FROM feedback_artifacts WHERE feedback_id=$1 AND sha256=$2 FOR UPDATE`, feedback, digest).Scan(&a.ID, &expired, &revision)
		if err == nil && expired {
			return a, RetentionExpiry{ErrScreenshotExpired, revision.Int64}
		}
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
	var expired bool
	var revision sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.storage_key,a.mime,a.bytes,a.sha256,a.removed_at IS NOT NULL OR COALESCE(a.expires_at<=statement_timestamp(),false),a.retention_revision FROM feedback_artifacts a
		JOIN preview_feedback f ON f.id=a.feedback_id JOIN static_preview_receipts p ON p.id=f.preview_id
		WHERE p.task_id=$1 AND p.id=$2 AND f.id=$3 AND a.id=$4`, task, preview, feedback, id).Scan(&a.ID, &a.Key, &a.MIME, &a.Bytes, &a.SHA256, &expired, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err == nil && expired {
		return a, RetentionExpiry{ErrScreenshotExpired, revision.Int64}
	}
	return a, err
}

func (s *Store) PurgeScreenshots(ctx context.Context, remove func(string) error) error {
	var after int64
	var firstErr error
	for {
		id, err := s.purgeNextScreenshot(ctx, after, remove)
		if errors.Is(err, sql.ErrNoRows) {
			return firstErr
		}
		if id == 0 {
			return err
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
		after = id
	}
}

func (s *Store) purgeNextScreenshot(ctx context.Context, after int64, remove func(string) error) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id, task, feedback int64
	var key, digest string
	err = tx.QueryRowContext(ctx, `SELECT a.id,p.task_id,f.id,a.storage_key,a.sha256 FROM feedback_artifacts a
		JOIN preview_feedback f ON f.id=a.feedback_id JOIN static_preview_receipts p ON p.id=f.preview_id
		WHERE a.expires_at<=statement_timestamp() AND a.removed_at IS NULL AND a.id>$1 ORDER BY a.id LIMIT 1
		FOR UPDATE OF a`, after).Scan(&id, &task, &feedback, &key, &digest)
	if err != nil {
		return 0, err
	}
	if key != fmt.Sprintf("tasks/%d/feedback/%d/%s.png", task, feedback, digest) {
		return id, errors.New("screenshot key differs from its receipt")
	}
	if err := remove(key); err != nil {
		return id, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE feedback_artifacts SET removed_at=now() WHERE id=$1", id); err != nil {
		return id, err
	}
	return id, tx.Commit()
}
