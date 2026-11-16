package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrRepositoryUnbound = errors.New("GitHub repository binding is pending")
var ErrWebhookRepository = errors.New("webhook repository differs from the project")

func (s *Store) RecordGitHubEvent(ctx context.Context, project, repository int64, delivery, event string, body []byte) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var bound sql.NullInt64
	err = tx.QueryRowContext(ctx, "SELECT github_repository_id FROM projects WHERE id=$1 FOR SHARE", project).Scan(&bound)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if !bound.Valid {
		return false, ErrRepositoryUnbound
	}
	if repository != bound.Int64 {
		return false, ErrWebhookRepository
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	res, err := tx.ExecContext(ctx, `INSERT INTO github_webhook_events(project_id,repository_id,delivery_id,event,body,sha256)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (project_id,delivery_id) DO NOTHING`, project, repository, delivery, event, body, digest)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 0 {
		var same bool
		err = tx.QueryRowContext(ctx, `SELECT repository_id=$3 AND event=$4 AND sha256=$5 FROM github_webhook_events
			WHERE project_id=$1 AND delivery_id=$2`, project, delivery, repository, event, digest).Scan(&same)
		if err != nil {
			return false, err
		}
		if !same {
			return false, ErrConflict
		}
	}
	return count == 1, tx.Commit()
}

func (s *Store) GitHubEvents(ctx context.Context, project, before int64) (json.RawMessage, error) {
	var events json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id DESC) FROM (
		SELECT id,repository_id,delivery_id,event,sha256,octet_length(body) AS bytes,received_at FROM github_webhook_events
		WHERE project_id=p.id AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT 100
	) v),'[]') FROM projects p WHERE p.id=$1`, project, before).Scan(&events)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return events, err
}

func (s *Store) GitHubEventBody(ctx context.Context, project int64, delivery string) ([]byte, error) {
	var body []byte
	err := s.db.QueryRowContext(ctx, "SELECT body FROM github_webhook_events WHERE project_id=$1 AND delivery_id=$2", project, delivery).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return body, err
}
