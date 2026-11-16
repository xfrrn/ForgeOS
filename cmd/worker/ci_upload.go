package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func queueCIFiles(ctx context.Context, tx *sql.Tx, key []byte, report json.RawMessage) error {
	var result store.Result
	if err := json.Unmarshal(report, &result); err != nil {
		return err
	}
	if result.Validation == nil || result.Validation.Run == nil || !result.Validation.Run.Valid() {
		return errors.New("cannot archive files without a valid CI run")
	}
	run := result.Validation.Run
	for name, digest := range run.Files() {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO ci_uploads(lease_hash,run_id,file,bytes,sha256) VALUES(?,?,?,?,?)`,
			key, run.ID, name, digest.Bytes, digest.SHA256); err != nil {
			return err
		}
	}
	return nil
}

func (w *worker) publishCIFiles(ctx context.Context) {
	for ctx.Err() == nil {
		var task, execution int64
		var key []byte
		var run, file string
		var digest validation.FileDigest
		err := w.journal.QueryRowContext(ctx, `SELECT a.task_id,a.execution_id,u.lease_hash,u.run_id,u.file,u.bytes,u.sha256
			FROM ci_uploads u JOIN attempts a ON a.lease_hash=u.lease_hash
			WHERE u.retry_at<=unixepoch() ORDER BY u.retry_at,a.execution_id,u.file LIMIT 1`).
			Scan(&task, &execution, &key, &run, &file, &digest.Bytes, &digest.SHA256)
		if err == nil {
			path := fmt.Sprintf("/internal/tasks/%d/executions/%d/ci/%s/%s", task, execution, run, file)
			if err = w.uploadCIFile(ctx, path, run, file, digest); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("execution %d CI file %s pending: %v", execution, file, err)
				_, err = w.journal.ExecContext(ctx, "UPDATE ci_uploads SET retry_at=unixepoch()+30 WHERE lease_hash=? AND file=?", key, file)
			} else {
				_, err = w.journal.ExecContext(ctx, "DELETE FROM ci_uploads WHERE lease_hash=? AND file=?", key, file)
			}
		}
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, sql.ErrNoRows) {
				log.Printf("CI archive queue: %v", err)
			}
			wait(ctx)
		}
	}
}

func (w *worker) uploadCIFile(ctx context.Context, path, run, file string, digest validation.FileDigest) error {
	dir := filepath.Join(w.workspaceRoot, run)
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() {
		return errors.New("local CI run directory unavailable")
	}
	content, err := archivedFile(dir, file, digest.Bytes)
	if err != nil {
		return err
	}
	if validation.DigestBytes(content) != digest {
		return errors.New("local file does not match the CI receipt")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, w.api+path, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("archive returned HTTP %d", resp.StatusCode)
	}
	return nil
}
