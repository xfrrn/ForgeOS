package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"forgeos/internal/validation"
)

// The sealed execution receipt determines which files belong to this task and runner.
func (s *Store) CIFile(ctx context.Context, task, execution int64, runID, file string) (validation.FileDigest, error) {
	var raw []byte
	var digest validation.FileDigest
	err := s.db.QueryRowContext(ctx, `SELECT result->'validation'->'run' FROM executions
		WHERE id=$1 AND task_id=$2 AND result->'validation'->'run'->>'id'=$3`, execution, task, runID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return digest, ErrNotFound
	}
	if err != nil {
		return digest, err
	}
	var run validation.Run
	if err = json.Unmarshal(raw, &run); err != nil {
		return digest, err
	}
	if !run.Valid() || run.ExecutionID != execution {
		return digest, ErrNotFound
	}
	digest, ok := run.Files()[file]
	if !ok {
		return digest, ErrNotFound
	}
	return digest, nil
}
