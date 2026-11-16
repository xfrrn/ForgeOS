package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

func (s *Store) OpenPreview(ctx context.Context, task, execution int64, commit, dir string, port, seconds int) (json.RawMessage, error) {
	return s.openPreview(ctx, task, execution, commit, dir, port, seconds, "local", nil)
}

func (s *Store) OpenApplicationPreview(ctx context.Context, task, execution int64, commit string, port, seconds int, start func(string, time.Time) error) (json.RawMessage, error) {
	return s.openPreview(ctx, task, execution, commit, "", port, seconds, "local-mock", start)
}

func (s *Store) openPreview(ctx context.Context, task, execution int64, commit, dir string, port, seconds int, environment string, start func(string, time.Time) error) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var matched int64
	err = tx.QueryRowContext(ctx, `SELECT e.id FROM executions e JOIN work_items t ON t.id=e.task_id
        WHERE t.kind='codex_issue' AND t.id=$1 AND e.id=$2 AND e.result->>'commit'=$3 AND e.result->'validation'->>'commit'=$3
        AND e.result->'validation'->>'exit_code'='0' AND e.result->>'exit_code'='0' FOR UPDATE OF e`, task, execution, commit).Scan(&matched)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	var id string
	var expires time.Time
	err = tx.QueryRowContext(ctx, `SELECT id,expires_at FROM static_previews WHERE execution_id=$1 AND directory=$2 AND port=$3 AND environment=$4 AND stopped_at IS NULL AND expires_at>clock_timestamp() ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, execution, dir, port, environment).Scan(&id, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		var key [24]byte
		if _, err = rand.Read(key[:]); err != nil {
			return nil, err
		}
		id = hex.EncodeToString(key[:])
		err = tx.QueryRowContext(ctx, `INSERT INTO static_previews(id,execution_id,directory,port,expires_at,environment) VALUES($1,$2,$3,$4,clock_timestamp()+$5*interval '1 second',$6) RETURNING expires_at`, id, execution, dir, port, seconds, environment).Scan(&expires)
	}
	if err != nil {
		return nil, err
	}
	if start != nil {
		if err = start(id, expires); err != nil {
			return nil, err
		}
		if !time.Now().Before(expires) {
			return nil, ErrConflict
		}
	}
	var out json.RawMessage
	if err = tx.QueryRowContext(ctx, `SELECT to_jsonb(r) FROM static_preview_receipts r WHERE id=$1`, id).Scan(&out); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func (s *Store) Previews(ctx context.Context, task, execution int64) (json.RawMessage, error) {
	var out json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY created_at DESC),'[]'::jsonb) FROM static_preview_receipts r WHERE task_id=$1 AND execution_id=$2`, task, execution).Scan(&out)
	return out, err
}

func (s *Store) StopPreview(ctx context.Context, task int64, id string) (string, error) {
	var environment string
	err := s.db.QueryRowContext(ctx, `UPDATE static_previews p SET stopped_at=CASE WHEN expires_at>now() THEN COALESCE(stopped_at,now()) ELSE stopped_at END FROM executions e WHERE p.id=$1 AND e.id=p.execution_id AND e.task_id=$2 RETURNING p.environment`, id, task).Scan(&environment)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return environment, err
}

type PreviewSource struct {
	Repo, Commit, Directory, Environment string
	Task, Execution                      int64
}

func (s *Store) PreviewSource(ctx context.Context, id string, port int) (PreviewSource, error) {
	var source PreviewSource
	err := s.db.QueryRowContext(ctx, `SELECT project.path,e.result->>'commit',p.directory,p.environment,e.task_id,e.id FROM static_previews p JOIN executions e ON e.id=p.execution_id JOIN work_items t ON t.id=e.task_id JOIN projects project ON project.id=t.project_id WHERE p.id=$1 AND p.port=$2 AND p.stopped_at IS NULL AND p.expires_at>now()`, id, port).Scan(&source.Repo, &source.Commit, &source.Directory, &source.Environment, &source.Task, &source.Execution)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return source, err
}
