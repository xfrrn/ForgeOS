package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Store) RepositoryInventory(ctx context.Context, task int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(i) FROM repository_inventories i WHERE task_id=$1`, task).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) SaveRepositoryInventory(ctx context.Context, task, execution int64, commit, origin string, files json.RawMessage) (json.RawMessage, error) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO repository_inventories(task_id,project_id,execution_id,commit_sha,github_origin,repository_id,files)
		SELECT t.id,p.id,e.id,e.result->>'commit',$4,p.github_repository_id,$5::jsonb
		FROM work_items t JOIN projects p ON p.id=t.project_id JOIN executions e ON e.task_id=t.id
		WHERE t.id=$1 AND t.kind='git_head' AND t.status='DONE' AND e.id=$2 AND e.result->>'commit'=$3
		AND (e.result->>'exit_code')::int=0 ON CONFLICT (task_id) DO NOTHING`, task, execution, commit, origin, string(files))
	if err != nil {
		return nil, err
	}
	return s.RepositoryInventory(ctx, task)
}
