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
	CREATE TABLE IF NOT EXISTS role_receipts (
 lease_hash BLOB NOT NULL, path TEXT NOT NULL, digest BLOB NOT NULL, report BLOB,
 buffered_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, settled_at TEXT, status INTEGER,
 conflicting_result TEXT, PRIMARY KEY(lease_hash,path)
 );
 CREATE TABLE IF NOT EXISTS role_process_receipts (
 instance INTEGER PRIMARY KEY REFERENCES harness_processes(instance), report BLOB,
 buffered_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, settled_at TEXT, status INTEGER,
 conflicting_report TEXT
 );
 CREATE TABLE IF NOT EXISTS role_claim_request (
 id INTEGER PRIMARY KEY CHECK(id=1), request_key TEXT NOT NULL, path TEXT NOT NULL,
 body BLOB NOT NULL, requested_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
 );
 CREATE TABLE IF NOT EXISTS role_assignment (
 id INTEGER PRIMARY KEY CHECK(id=1), request_key TEXT NOT NULL, lease_hash BLOB NOT NULL,
 run_id TEXT NOT NULL, response BLOB NOT NULL, started INTEGER NOT NULL DEFAULT 0,
 received_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
 );
 CREATE TABLE IF NOT EXISTS role_native_attempts (
 instance INTEGER PRIMARY KEY, resumed_from INTEGER NOT NULL
 );
 CREATE TABLE IF NOT EXISTS role_native_handoffs (
 lease_hash BLOB NOT NULL, turn INTEGER NOT NULL, missing_instance INTEGER NOT NULL, missing_session TEXT NOT NULL,
 PRIMARY KEY(lease_hash,turn,missing_instance)
 );
 CREATE TABLE IF NOT EXISTS role_native_turns (
 lease_hash BLOB NOT NULL, turn INTEGER NOT NULL, harness TEXT NOT NULL, version TEXT NOT NULL,
 workspace TEXT NOT NULL, prompt TEXT, contract TEXT, instance INTEGER,
 PRIMARY KEY(lease_hash,turn)
 );
 CREATE TABLE IF NOT EXISTS role_turn_results (
 lease_hash BLOB NOT NULL, turn INTEGER NOT NULL, session_id TEXT, result BLOB,
 completed_at TEXT, PRIMARY KEY(lease_hash,turn)
 );
 CREATE TABLE IF NOT EXISTS candidate_finalizations (
 lease_hash BLOB PRIMARY KEY REFERENCES attempts(lease_hash), checkpoint BLOB NOT NULL
 );
 CREATE TABLE IF NOT EXISTS parked_questions (
		lease_hash BLOB PRIMARY KEY REFERENCES attempts(lease_hash), checkpoint BLOB NOT NULL
	);
	CREATE TABLE IF NOT EXISTS harness_session_handoffs (
		instance INTEGER PRIMARY KEY REFERENCES harness_processes(instance), previous_session TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS harness_app_turns (
		instance INTEGER PRIMARY KEY REFERENCES harness_processes(instance), turn_id TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS harness_processes_attempt ON harness_processes(lease_hash,instance);
	CREATE TABLE IF NOT EXISTS harness_tool_events (
		instance INTEGER NOT NULL REFERENCES harness_processes(instance), seq INTEGER NOT NULL, call INTEGER NOT NULL,
		tool TEXT NOT NULL, phase TEXT NOT NULL, at INTEGER NOT NULL DEFAULT (unixepoch()), PRIMARY KEY(instance,seq)
	);
	CREATE TABLE IF NOT EXISTS harness_model_output (
		instance INTEGER PRIMARY KEY REFERENCES harness_processes(instance), sequence INTEGER NOT NULL, text TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS harness_tool_output (
		instance INTEGER PRIMARY KEY REFERENCES harness_processes(instance), seq INTEGER NOT NULL, call INTEGER NOT NULL,
		tool TEXT NOT NULL, text TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS harness_output_redaction (
		lease_hash BLOB NOT NULL, stream TEXT NOT NULL, private_key INTEGER NOT NULL, PRIMARY KEY(lease_hash,stream)
	);
	CREATE TABLE IF NOT EXISTS ci_uploads (
		lease_hash BLOB NOT NULL REFERENCES attempts(lease_hash), run_id TEXT NOT NULL,
		file TEXT NOT NULL, bytes INTEGER NOT NULL, sha256 TEXT NOT NULL, retry_at INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (lease_hash,file)
	);
	CREATE INDEX IF NOT EXISTS pending_attempts ON attempts(execution_id) WHERE report IS NOT NULL`)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = prepareProcessArchives(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (w *worker) executeAndReport(ctx context.Context, job store.Job, requested time.Time) error {
	return w.runAttempt(ctx, job, requested, nil)
}

func (w *worker) runAttempt(ctx context.Context, job store.Job, requested time.Time, saved *parkedCheckpoint) error {
	key := sha256.Sum256([]byte(job.LeaseToken))
	unlock, err := w.lockAttempt(job.ExecutionID)
	if err != nil {
		return err
	}
	defer unlock()
	if saved != nil && (saved.Finalization != nil || saved.CompletedTip != "") {
		// The previous owner can advance or settle the candidate between queue discovery and this lock.
		saved, err = w.readFinalization(ctx, job.ExecutionID)
		if err != nil || saved == nil {
			return err
		}
		job = saved.Job
	} else if saved == nil {
		_, err = w.journal.ExecContext(ctx, `INSERT INTO attempts(lease_hash,execution_id,task_id,workspace) VALUES(?,?,?,?)`,
			key[:], job.ExecutionID, job.TaskID, job.Workspace)
		if err != nil {
			return fmt.Errorf("record claim for execution %d: %w", job.ExecutionID, err)
		}
	}
	recovery := checkpointRecovery{saved: saved, waiting: saved != nil, leaveUnsettled: saved != nil}
	result := w.executeWithLease(ctx, job, requested, &recovery)
	if recovery.leaveUnsettled || recovery.waiting && ctx.Err() != nil {
		log.Printf("execution %d left at recorded checkpoint: %s", job.ExecutionID, result.Error)
		return nil
	}
	if job.RecordProcesses {
		result.Processes, err = w.processHistory(context.WithoutCancel(ctx), key[:])
		if err != nil {
			return fmt.Errorf("read process receipts for execution %d: %w", job.ExecutionID, err)
		}
		if result.Processes != nil {
			if job.RecordArchiveDigests {
				if err = w.attachProcessArchives(context.WithoutCancel(ctx), key[:], *result.Processes); err != nil {
					return fmt.Errorf("read process archives for execution %d: %w", job.ExecutionID, err)
				}
			}
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
	writeCtx := context.WithoutCancel(ctx)
	tx, err := w.journal.BeginTx(writeCtx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	buffered, err := tx.ExecContext(writeCtx, `UPDATE attempts SET buffered_at=CURRENT_TIMESTAMP,
		exit_code=?,candidate=?,session_id=?,report=? WHERE lease_hash=? AND buffered_at IS NULL`, result.ExitCode, result.Commit, result.SessionID, report, key[:])
	if err != nil {
		return fmt.Errorf("buffer result for execution %d: %w", job.ExecutionID, err)
	}
	count, err := buffered.RowsAffected()
	if err != nil || count == 0 {
		return err
	}
	if _, err = tx.ExecContext(writeCtx, `DELETE FROM parked_questions WHERE lease_hash=?`, key[:]); err != nil {
		return err
	}
	if _, err = tx.ExecContext(writeCtx, "DELETE FROM candidate_finalizations WHERE lease_hash=?", key[:]); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return w.sendReceipt(ctx, key[:], job.ExecutionID, report)
}

func (w *worker) processHistory(ctx context.Context, key []byte) (*store.ProcessHistory, error) {
	rows, err := w.journal.QueryContext(ctx, readHarnessProcess+` WHERE lease_hash=? ORDER BY instance`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var history store.ProcessHistory
	for rows.Next() {
		p, err := scanHarnessProcess(rows)
		if err != nil {
			return nil, err
		}
		if p.Exited == nil {
			return nil, errors.New("harness process has no exit receipt")
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
		confirmCtx := context.WithoutCancel(ctx)
		resp, err := w.postJSON(confirmCtx, path, report)
		var status int
		var archive bool
		if err == nil {
			status = resp.StatusCode
			archive = resp.Header.Get("ForgeOS-CI-Archive") == "1" && status == http.StatusNoContent
			resp.Body.Close()
		}
		outcome := ""
		switch status {
		case http.StatusNoContent:
			outcome = "confirmed"
		case http.StatusConflict:
			outcome = "conflict"
		}
		if outcome != "" {
			tx, err := w.journal.BeginTx(confirmCtx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			if archive {
				if err = queueCIFiles(confirmCtx, tx, key, report); err != nil {
					return err
				}
			}
			_, err = tx.ExecContext(confirmCtx, `UPDATE attempts SET outcome=?,settled_at=CURRENT_TIMESTAMP,
				conflicting_result=CASE WHEN ?='conflict' THEN json_remove(CAST(report AS TEXT),'$.lease_token') END,
				report=NULL WHERE lease_hash=? AND report IS NOT NULL`, outcome, outcome, key)
			if err == nil {
				err = tx.Commit()
			}
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
