package store

import (
	"context"
	"database/sql"
	"errors"

	"forgeos/internal/localrepo"
)

// Older scans retained the Git blob ID on size_limit records. Each backfill is atomic and independently retryable.
func (s *Store) collectDocumentParts(ctx context.Context, project int64, repo string) error {
	var after int64
	var firstErr error
	for ctx.Err() == nil {
		id, err := s.collectNextDocument(ctx, project, after, repo)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if id == 0 {
			return firstErr
		}
		after = id
	}
	return ctx.Err()
}

func (s *Store) collectNextDocument(ctx context.Context, project, after int64, repo string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	var blob string
	err = tx.QueryRowContext(ctx, `SELECT id,blob FROM repository_document_changes WHERE project_id=$1 AND id>$2
		AND problem='size_limit' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`, project, after).Scan(&id, &blob)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	result, err := localrepo.ReadDocumentParts(ctx, repo, blob, func(part int, offset int64, text string) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO repository_document_parts(change_id,part,byte_offset,content) VALUES($1,$2,$3,$4)`, id, part, offset, text)
		return err
	})
	if errors.Is(err, localrepo.ErrDocumentBlobMissing) {
		return id, nil
	}
	if err != nil {
		return id, err
	}
	if result.Problem != "" {
		if _, err = tx.ExecContext(ctx, `DELETE FROM repository_document_parts WHERE change_id=$1`, id); err != nil {
			return id, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE repository_document_changes SET problem=$2 WHERE id=$1`, id, result.Problem)
	} else {
		_, err = tx.ExecContext(ctx, `WITH finished AS (
			UPDATE repository_document_changes SET problem='partitioned',part_count=$2,byte_length=$3,file_sha256=$4 WHERE id=$1 RETURNING id,project_id)
			INSERT INTO material_curations(project_id,document_id,document_part)
			SELECT f.project_id,f.id,p.part FROM finished f JOIN repository_document_parts p ON p.change_id=f.id ORDER BY p.part`, id, result.Count, result.Bytes, result.Digest)
	}
	if err != nil {
		return id, err
	}
	return id, tx.Commit()
}
