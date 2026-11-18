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

func archiveTransfer(ctx context.Context, tx *sql.Tx, task, execution, instance int64, lock bool) (ArchiveTransfer, error) {
	var a ArchiveTransfer
	var raw []byte
	var expired bool
	query := `SELECT p->'archive',a.received,a.verified,a.expires_at<=clock_timestamp() FROM process_archive_transfers a JOIN executions e ON e.id=a.execution_id
 CROSS JOIN LATERAL jsonb_array_elements(e.result->'processes') p WHERE e.task_id=$1 AND e.id=$2 AND a.instance=$3 AND (p->>'instance')::bigint=a.instance`
	if lock {
		query += " FOR UPDATE OF a"
	}
	err := tx.QueryRowContext(ctx, query, task, execution, instance).Scan(&raw, &a.Received, &a.Verified, &expired)
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

func (s *Store) ArchiveUploadStatus(ctx context.Context, task, execution, instance int64) (ArchiveTransfer, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ArchiveTransfer{}, err
	}
	defer tx.Rollback()
	return archiveTransfer(ctx, tx, task, execution, instance, false)
}

func archiveChunks(ctx context.Context, tx *sql.Tx, execution, instance int64, visit func([]byte) error) error {
	rows, err := tx.QueryContext(ctx, `SELECT start_byte,content FROM process_archive_chunks WHERE execution_id=$1 AND instance=$2 ORDER BY start_byte`, execution, instance)
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

func verifyArchiveChunks(ctx context.Context, tx *sql.Tx, execution, instance int64, want ProcessArchive) error {
	var total, count int64
	var digest string
	hash := sha256.New()
	completeLine := true
	err := archiveChunks(ctx, tx, execution, instance, func(block []byte) error {
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

func (s *Store) PutArchiveChunk(ctx context.Context, task, execution, instance, offset int64, data []byte) error {
	if offset < 0 || offset%ProcessArchiveChunkSize != 0 || len(data) == 0 || len(data) > ProcessArchiveChunkSize {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	a, err := archiveTransfer(ctx, tx, task, execution, instance, true)
	if err != nil {
		return err
	}
	if offset > a.Received || offset >= a.Bytes || int64(len(data)) != min(int64(ProcessArchiveChunkSize), a.Bytes-offset) {
		return ErrConflict
	}
	if offset < a.Received {
		var same bool
		err = tx.QueryRowContext(ctx, `SELECT content=$4 FROM process_archive_chunks WHERE execution_id=$1 AND instance=$2 AND start_byte=$3`, execution, instance, offset, data).Scan(&same)
		if errors.Is(err, sql.ErrNoRows) || err == nil && !same {
			return ErrConflict
		}
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO process_archive_chunks(execution_id,instance,start_byte,content) VALUES($1,$2,$3,$4)`, execution, instance, offset, data)
	if err != nil {
		return err
	}
	next := offset + int64(len(data))
	final := next == a.Bytes
	if final {
		err = verifyArchiveChunks(ctx, tx, execution, instance, a.ProcessArchive)
		if errors.Is(err, ErrConflict) {
			if _, err = tx.ExecContext(ctx, `DELETE FROM process_archive_chunks WHERE execution_id=$1 AND instance=$2`, execution, instance); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE process_archive_transfers SET received=0 WHERE execution_id=$1 AND instance=$2`, execution, instance); err != nil {
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
	_, err = tx.ExecContext(ctx, `UPDATE process_archive_transfers SET received=$3,verified=$4 WHERE execution_id=$1 AND instance=$2`, execution, instance, next, final)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DownloadProcessArchive(ctx context.Context, task, execution, instance int64, start func(ProcessArchive) io.Writer) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	a, err := archiveTransfer(ctx, tx, task, execution, instance, false)
	if err != nil {
		return err
	}
	if !a.Verified {
		return ErrProcessArchivePending
	}
	if err = verifyArchiveChunks(ctx, tx, execution, instance, a.ProcessArchive); err != nil {
		return err
	}
	out := start(a.ProcessArchive)
	return archiveChunks(ctx, tx, execution, instance, func(b []byte) error { _, err := out.Write(b); return err })
}

func (s *Store) PurgeProcessArchiveChunks(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM process_archive_chunks c USING process_archive_transfers a
 WHERE a.execution_id=c.execution_id AND a.instance=c.instance AND a.expires_at<=clock_timestamp()`)
	return err
}
