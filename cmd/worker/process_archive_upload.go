package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"forgeos/internal/store"
)

type archiveEntryReader struct {
	ctx                     context.Context
	db                      *sql.DB
	instance, seq, consumed int64
}

func (r *archiveEntryReader) Read(dst []byte) (int, error) {
	var part []byte
	var length int64
	err := r.db.QueryRowContext(r.ctx, `SELECT substr(entry,?,?),length(entry) FROM harness_log_entries WHERE instance=? AND seq=?`,
		r.consumed+1, len(dst), r.instance, r.seq).Scan(&part, &length)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, io.EOF
	}
	if err != nil {
		return 0, err
	}
	n := copy(dst, part)
	r.consumed += int64(n)
	if r.consumed == length {
		r.seq++
		r.consumed = 0
	}
	if n == 0 && len(dst) != 0 {
		return 0, errors.New("empty node archive entry")
	}
	return n, nil
}

func (w *worker) publishProcessArchives(ctx context.Context) {
	for ctx.Err() == nil {
		var task, execution, instance, size int64
		err := w.journal.QueryRowContext(ctx, `SELECT a.task_id,a.execution_id,p.instance,l.bytes FROM process_log_uploads u
 JOIN harness_processes p USING(instance) JOIN attempts a ON a.lease_hash=p.lease_hash JOIN harness_log_archives l ON l.instance=p.instance
 WHERE u.retry_at<=unixepoch() ORDER BY u.retry_at,p.instance LIMIT 1`).Scan(&task, &execution, &instance, &size)
		if err == nil {
			err = w.uploadProcessArchive(ctx, task, execution, instance, size)
			if err == nil {
				_, err = w.journal.ExecContext(ctx, "DELETE FROM process_log_uploads WHERE instance=?", instance)
			} else if ctx.Err() == nil {
				log.Printf("execution %d process %d archive pending: %v", execution, instance, err)
				_, err = w.journal.ExecContext(ctx, "UPDATE process_log_uploads SET retry_at=unixepoch()+30 WHERE instance=?", instance)
			}
		}
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, sql.ErrNoRows) {
				log.Printf("process archive queue: %v", err)
			}
			wait(ctx)
		}
	}
}

func (w *worker) uploadProcessArchive(ctx context.Context, task, execution, instance, size int64) error {
	path := fmt.Sprintf("%s/internal/tasks/%d/executions/%d/processes/%d/archive", w.api, task, execution, instance)
	head, err := http.NewRequestWithContext(ctx, http.MethodHead, path, nil)
	if err != nil {
		return err
	}
	reply, err := w.authenticatedRequest(head)
	if err != nil {
		return err
	}
	reply.Body.Close()
	if reply.StatusCode == http.StatusGone {
		return nil
	}
	if reply.StatusCode != http.StatusNoContent {
		return fmt.Errorf("process archive status HTTP %d", reply.StatusCode)
	}
	offset, err := strconv.ParseInt(reply.Header.Get("ForgeOS-Archive-Offset"), 10, 64)
	if err != nil || offset < 0 || offset > size || offset < size && offset%store.ProcessArchiveChunkSize != 0 {
		return errors.New("invalid central archive offset")
	}
	if offset == size {
		return nil
	}
	source := &archiveEntryReader{ctx: ctx, db: w.journal, instance: instance, seq: 1}
	if offset > 0 {
		// SQLite can locate a BLOB by its length without reading the acknowledged prefix.
		err = w.journal.QueryRowContext(ctx, `WITH spans AS (SELECT seq,length(entry) AS size,
			sum(length(entry)) OVER(ORDER BY seq) AS end_byte FROM harness_log_entries WHERE instance=?)
			SELECT seq,?-(end_byte-size) FROM spans WHERE end_byte>? ORDER BY seq LIMIT 1`, instance, offset, offset).Scan(&source.seq, &source.consumed)
		if err != nil {
			return err
		}
	}
	block := make([]byte, store.ProcessArchiveChunkSize)
	client := *w.client
	client.Timeout = 5 * time.Minute
	for offset < size {
		part := block[:min(int64(len(block)), size-offset)]
		if _, err = io.ReadFull(source, part); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, path+"?offset="+strconv.FormatInt(offset, 10), bytes.NewReader(part))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		reply, err = w.authenticatedWithClient(req, &client)
		if err != nil {
			return err
		}
		reply.Body.Close()
		if reply.StatusCode == http.StatusGone {
			return nil
		}
		if reply.StatusCode != http.StatusNoContent {
			return fmt.Errorf("process archive block HTTP %d", reply.StatusCode)
		}
		offset += int64(len(part))
	}
	return nil
}
