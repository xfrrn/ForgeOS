package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

const ProcessArchiveChunkSize = 256 << 10

var ErrProcessArchiveExpired = errors.New("process archive retention period elapsed")
var ErrProcessArchivePending = errors.New("process archive upload pending")

type ArchiveTransfer struct {
	ProcessArchive
	Received int64
	Verified bool
}

// Execution archives are scoped to a task. Role uploads also require the configured project and role.
type ArchiveTarget struct {
	Task, Execution, Instance int64
	RoleProcess, Project      int64
	Role                      string
}

type archiveStorage struct {
	transfers, chunks, key string
	id, instance           int64
}

func (t ArchiveTarget) storage() archiveStorage {
	if t.RoleProcess != 0 {
		return archiveStorage{"role_archive_transfers", "role_archive_chunks", "process_id", t.RoleProcess, t.Instance}
	}
	return archiveStorage{"process_archive_transfers", "process_archive_chunks", "execution_id", t.Execution, t.Instance}
}

func archiveTransfer(ctx context.Context, tx *sql.Tx, t ArchiveTarget, lock bool) (ArchiveTransfer, error) {
	var a ArchiveTransfer
	var raw []byte
	var expired bool
	query := `SELECT p->'archive',a.received,a.verified,a.expires_at<=clock_timestamp() FROM process_archive_transfers a JOIN executions e ON e.id=a.execution_id
 CROSS JOIN LATERAL jsonb_array_elements(e.result->'processes') p WHERE e.task_id=$1 AND e.id=$2 AND a.instance=$3 AND (p->>'instance')::bigint=a.instance`
	args := []any{t.Task, t.Execution, t.Instance}
	if t.RoleProcess != 0 {
		query = `SELECT p.process->'archive',a.received,a.verified,a.expires_at<=clock_timestamp()
            FROM role_archive_transfers a JOIN management_processes p ON p.id=a.process_id
            WHERE a.process_id=$1 AND a.instance=$2`
		args = []any{t.RoleProcess, t.Instance}
		if t.Role != "" || t.Project != 0 || lock {
			query += " AND a.project_id=$3 AND a.role=$4"
			args = append(args, t.Project, t.Role)
		}
	}
	if lock {
		query += " FOR UPDATE OF a"
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&raw, &a.Received, &a.Verified, &expired)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, err
	}
	if expired {
		return a, ErrProcessArchiveExpired
	}
	err = json.Unmarshal(raw, &a.ProcessArchive)
	return a, err
}

func (s *Store) ArchiveUploadStatus(ctx context.Context, t ArchiveTarget) (ArchiveTransfer, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ArchiveTransfer{}, err
	}
	defer tx.Rollback()
	return archiveTransfer(ctx, tx, t, false)
}

func archiveChunks(ctx context.Context, tx *sql.Tx, storage archiveStorage, visit func([]byte) error) error {
	rows, err := tx.QueryContext(ctx, `SELECT start_byte,content FROM `+storage.chunks+` WHERE `+storage.key+`=$1 AND instance=$2 ORDER BY start_byte`, storage.id, storage.instance)
	if err != nil {
		return err
	}
	defer rows.Close()
	var total int64
	for rows.Next() {
		var offset int64
		var block []byte
		if err = rows.Scan(&offset, &block); err != nil {
			return err
		}
		if offset != total || len(block) == 0 || len(block) > ProcessArchiveChunkSize {
			return ErrConflict
		}
		if err = visit(block); err != nil {
			return err
		}
		total += int64(len(block))
	}
	return rows.Err()
}

func verifyArchiveChunks(ctx context.Context, tx *sql.Tx, storage archiveStorage, want ProcessArchive) error {
	var total, count int64
	var digest string
	hash := sha256.New()
	completeLine := true
	err := archiveChunks(ctx, tx, storage, func(block []byte) error {
		total += int64(len(block))
		for len(block) > 0 {
			i := bytes.IndexByte(block, '\n')
			if i < 0 {
				hash.Write(block)
				completeLine = false
				break
			}
			hash.Write(block[:i+1])
			digest = hex.EncodeToString(hash.Sum(nil))
			count++
			hash.Reset()
			hash.Write([]byte(digest))
			block = block[i+1:]
			completeLine = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !completeLine || total != want.Bytes || count != want.Records || digest != want.Digest {
		return ErrConflict
	}
	return nil
}

func (s *Store) PutArchiveChunk(ctx context.Context, t ArchiveTarget, offset int64, data []byte) error {
	if offset < 0 || offset%ProcessArchiveChunkSize != 0 || len(data) == 0 || len(data) > ProcessArchiveChunkSize {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	storage := t.storage()
	a, err := archiveTransfer(ctx, tx, t, true)
	if err != nil {
		return err
	}
	if offset > a.Received || offset >= a.Bytes || int64(len(data)) != min(int64(ProcessArchiveChunkSize), a.Bytes-offset) {
		return ErrConflict
	}
	if offset < a.Received {
		var same bool
		err = tx.QueryRowContext(ctx, `SELECT content=$4 FROM `+storage.chunks+` WHERE `+storage.key+`=$1 AND instance=$2 AND start_byte=$3`, storage.id, storage.instance, offset, data).Scan(&same)
		if errors.Is(err, sql.ErrNoRows) || err == nil && !same {
			return ErrConflict
		}
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO `+storage.chunks+`(`+storage.key+`,instance,start_byte,content) VALUES($1,$2,$3,$4)`, storage.id, storage.instance, offset, data)
	if err != nil {
		return err
	}
	next := offset + int64(len(data))
	final := next == a.Bytes
	if final {
		err = verifyArchiveChunks(ctx, tx, storage, a.ProcessArchive)
		if errors.Is(err, ErrConflict) {
			if _, err = tx.ExecContext(ctx, `DELETE FROM `+storage.chunks+` WHERE `+storage.key+`=$1 AND instance=$2`, storage.id, storage.instance); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE `+storage.transfers+` SET received=0 WHERE `+storage.key+`=$1 AND instance=$2`, storage.id, storage.instance); err != nil {
				return err
			}
			if err = tx.Commit(); err != nil {
				return err
			}
			return ErrConflict
		}
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE `+storage.transfers+` SET received=$3,verified=$4 WHERE `+storage.key+`=$1 AND instance=$2`, storage.id, storage.instance, next, final)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DownloadProcessArchive(ctx context.Context, t ArchiveTarget, start func(ProcessArchive) io.Writer) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	storage := t.storage()
	a, err := archiveTransfer(ctx, tx, t, false)
	if err != nil {
		return err
	}
	if !a.Verified {
		return ErrProcessArchivePending
	}
	if err = verifyArchiveChunks(ctx, tx, storage, a.ProcessArchive); err != nil {
		return err
	}
	out := start(a.ProcessArchive)
	return archiveChunks(ctx, tx, storage, func(b []byte) error { _, err := out.Write(b); return err })
}

func (s *Store) PurgeProcessArchiveChunks(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM process_archive_chunks c USING process_archive_transfers a
 WHERE a.execution_id=c.execution_id AND a.instance=c.instance AND a.expires_at<=clock_timestamp()`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM role_archive_chunks c USING role_archive_transfers a WHERE a.process_id=c.process_id AND a.expires_at<=clock_timestamp()`)
	return err
}
