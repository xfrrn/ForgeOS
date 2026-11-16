package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"forgeos/internal/store"
	_ "modernc.org/sqlite"
)

// Each identity and control-plane origin has its own recovery history.
func (w *worker) openJournal(ctx context.Context) (*sql.DB, error) {
	dir := os.Getenv("FORGEOS_WORKER_STATE_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".forgeos", "worker")
	}
	if !filepath.IsAbs(dir) {
		return nil, errors.New("worker state directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	for _, root := range []string{w.repoRoot, w.workspaceRoot} {
		if rel, err := filepath.Rel(root, dir); err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return nil, errors.New("worker state must be outside repositories and workspaces")
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("worker state directory must be private to the service account")
	}
	scope := sha256.Sum256([]byte(w.api + "\x00" + w.id))
	path := filepath.Join(dir, fmt.Sprintf("%x.sqlite", scope))
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return nil, errors.New("worker journal must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = file.Close(); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/"),
		RawQuery: "_pragma=busy_timeout(5000)&_pragma=secure_delete(ON)&_pragma=synchronous(FULL)"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS attempts (
		lease_hash BLOB PRIMARY KEY, execution_id INTEGER NOT NULL, task_id INTEGER NOT NULL, workspace TEXT NOT NULL,
		claimed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, buffered_at TEXT, settled_at TEXT,
		exit_code INTEGER, candidate TEXT, session_id TEXT, outcome TEXT, report BLOB, conflicting_result TEXT
	);
	CREATE TABLE IF NOT EXISTS harness_processes (
		instance INTEGER PRIMARY KEY, lease_hash BLOB NOT NULL REFERENCES attempts(lease_hash), harness TEXT NOT NULL, version TEXT NOT NULL,
		pid INTEGER NOT NULL, started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, session_id TEXT, exited_at TEXT, exit_code INTEGER
	);
	CREATE INDEX IF NOT EXISTS harness_processes_attempt ON harness_processes(lease_hash,instance);
	CREATE INDEX IF NOT EXISTS pending_attempts ON attempts(execution_id) WHERE report IS NOT NULL`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (w *worker) executeAndReport(ctx context.Context, job store.Job, requested time.Time) error {
	key := sha256.Sum256([]byte(job.LeaseToken))
	_, err := w.journal.ExecContext(ctx, `INSERT INTO attempts(lease_hash,execution_id,task_id,workspace) VALUES(?,?,?,?)`,
		key[:], job.ExecutionID, job.TaskID, job.Workspace)
	if err != nil {
		return fmt.Errorf("record claim for execution %d: %w", job.ExecutionID, err)
	}
	result := w.executeWithLease(ctx, job, requested)
	if job.RecordProcesses {
		result.Processes, err = w.processHistory(context.WithoutCancel(ctx), key[:])
		if err != nil {
			return fmt.Errorf("read process receipts for execution %d: %w", job.ExecutionID, err)
		}
		if result.Processes != nil {
			first := (*result.Processes)[0]
			result.Harness, result.HarnessVersion = first.Harness, first.Version
		}
	}
	report, err := json.Marshal(struct {
		LeaseToken string `json:"lease_token"`
		store.Result
	}{job.LeaseToken, result})
	if err != nil {
		return err
	}
	// Finish writing the receipt even when shutdown interrupted the harness.
	_, err = w.journal.ExecContext(context.WithoutCancel(ctx), `UPDATE attempts SET buffered_at=CURRENT_TIMESTAMP,
		exit_code=?,candidate=?,session_id=?,report=? WHERE lease_hash=?`, result.ExitCode, result.Commit, result.SessionID, report, key[:])
	if err != nil {
		return fmt.Errorf("buffer result for execution %d: %w", job.ExecutionID, err)
	}
	return w.sendReceipt(ctx, key[:], job.ExecutionID, report)
}

func (w *worker) processHistory(ctx context.Context, key []byte) (*store.ProcessHistory, error) {
	rows, err := w.journal.QueryContext(ctx, `SELECT instance,pid,harness,version,coalesce(session_id,''),started_at,exited_at,exit_code
		FROM harness_processes WHERE lease_hash=? ORDER BY instance`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history store.ProcessHistory
	for rows.Next() {
		var p store.HarnessProcess
		var start, end string
		if err := rows.Scan(&p.Instance, &p.PID, &p.Harness, &p.Version, &p.Session, &start, &end, &p.ExitCode); err != nil {
			return nil, err
		}
		if p.Started, err = time.Parse(time.DateTime, start); err != nil {
			return nil, err
		}
		if p.Exited, err = time.Parse(time.DateTime, end); err != nil {
			return nil, err
		}
		history = append(history, p)
	}
	if err := rows.Err(); err != nil || len(history) == 0 {
		return nil, err
	}
	return &history, nil
}

func (w *worker) replayReceipts(ctx context.Context) error {
	for ctx.Err() == nil {
		var key, report []byte
		var id int64
		err := w.journal.QueryRowContext(ctx, `SELECT lease_hash,execution_id,report FROM attempts WHERE report IS NOT NULL ORDER BY execution_id LIMIT 1`).Scan(&key, &id, &report)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := w.sendReceipt(ctx, key, id, report); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (w *worker) sendReceipt(ctx context.Context, key []byte, id int64, report json.RawMessage) error {
	path := fmt.Sprintf("/internal/executions/%d/result", id)
	for {
		status, _ := w.post(context.WithoutCancel(ctx), path, report, nil)
		outcome := ""
		switch status {
		case http.StatusNoContent:
			outcome = "confirmed"
		case http.StatusConflict:
			outcome = "conflict"
		}
		if outcome != "" {
			_, err := w.journal.ExecContext(context.WithoutCancel(ctx), `UPDATE attempts SET outcome=?,settled_at=CURRENT_TIMESTAMP,
				conflicting_result=CASE WHEN ?='conflict' THEN json_remove(CAST(report AS TEXT),'$.lease_token') END,
				report=NULL WHERE lease_hash=? AND report IS NOT NULL`, outcome, outcome, key)
			if err == nil {
				log.Printf("execution %d result %s", id, outcome)
			}
			return err
		}
		if ctx.Err() != nil || !temporary(status) {
			return fmt.Errorf("execution %d result remains buffered (HTTP %d)", id, status)
		}
		log.Printf("execution %d result pending (HTTP %d), retrying", id, status)
		wait(ctx)
	}
}
