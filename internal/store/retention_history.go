package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// A zero revision denotes a file that expired before policy history was recorded.
type RetentionExpiry struct {
	Cause    error
	Revision int64
}

func (e RetentionExpiry) Error() string { return e.Cause.Error() }
func (e RetentionExpiry) Unwrap() error { return e.Cause }

func recordRetentionChange(ctx context.Context, tx *sql.Tx, project int64, scope string, before, after []byte) (sql.NullInt64, error) {
	var id sql.NullInt64
	err := tx.QueryRowContext(ctx, `INSERT INTO retention_revisions(project_id,scope,actor,previous_policy,policy)
		SELECT $1,$2,'owner',$3::jsonb,$4::jsonb WHERE $3::jsonb<>$4::jsonb RETURNING id`, project, scope, string(before), string(after)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return id, nil
	}
	return id, err
}

func (s *Store) RetentionHistory(ctx context.Context, project, before int64) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id DESC) FROM (
		SELECT * FROM retention_revisions WHERE project_id=p.id AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT 100
	) r),'[]') FROM projects p WHERE p.id=$1`, project, before).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return result, err
}
