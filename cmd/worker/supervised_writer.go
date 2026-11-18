package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

var errWriterDelegated = errors.New("writer transferred to an execution process")
var errWriterBusy = errors.New("another process owns this execution")

const (
	writerBusyExit  = 75
	writerErrorExit = 70
)

func (w *worker) marshalWriterCheckpoint(q parkedCheckpoint) ([]byte, error) {
	// Older supervised checkpoints all belonged to App Server writers.
	q.SupervisedExec = w.superviseWriters && !w.codexAppServer && q.Job.Spec != nil && q.Job.Spec.Harness != "claude"
	return json.Marshal(q)
}

func (w *worker) recordInitialTurn(ctx context.Context, q *parkedCheckpoint, recovery *checkpointRecovery) error {
	var err error
	q.Trees, err = checkpointTrees(ctx, q.Job.Workspace)
	if err != nil {
		return err
	}
	if !w.superviseWriters || !q.Job.RecordAppTurn || q.Job.Spec.Harness == "claude" {
		err := w.saveFinalization(ctx, q)
		recovery.waiting = err == nil
		return err
	}
	raw, err := w.marshalWriterCheckpoint(*q)
	if err != nil {
		return err
	}
	tx, err := w.journal.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	key := sha256.Sum256([]byte(q.Job.LeaseToken))
	if _, err = tx.ExecContext(ctx, "INSERT INTO candidate_finalizations(lease_hash,checkpoint) VALUES(?,?)", key[:], raw); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO supervised_writers(lease_hash) VALUES(?)", key[:]); err != nil {
		return err
	}
	err = tx.Commit()
	recovery.delegated, recovery.waiting = err == nil, err == nil
	return err
}

func (w *worker) supervisedWriters(ctx context.Context) ([]int64, error) {
	rows, err := w.journal.QueryContext(ctx, "SELECT a.execution_id FROM supervised_writers JOIN attempts a USING(lease_hash) ORDER BY a.execution_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (w *worker) writerAssignment(ctx context.Context, id int64) ([]byte, error) {
	var key []byte
	err := w.journal.QueryRowContext(ctx, "SELECT lease_hash FROM supervised_writers JOIN attempts USING(lease_hash) WHERE execution_id=? AND settled_at IS NULL", id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return key, err
}

// The execution process owns the native pipes and the lease. Each Daemon counts it against one slot.
func (w *worker) followWriter(ctx context.Context, id int64) error {
	attached := false
	retried := false
	for ctx.Err() == nil {
		unlock, err := w.lockAttempt(id)
		if errors.Is(err, errWriterBusy) {
			if !attached {
				log.Printf("following active writer for execution %d", id)
				attached = true
			}
			wait(ctx)
			continue
		}
		if err != nil {
			return err
		}
		key, err := w.writerAssignment(ctx, id)
		unlock()
		if err != nil || key == nil {
			return err
		}
		bin, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(bin, "run-saved-writer", strconv.FormatInt(id, 10))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-ctx.Done():
			return nil
		case err := <-done:
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == writerBusyExit {
				continue
			}
			if exit != nil && exit.ExitCode() != writerErrorExit && !retried {
				log.Printf("execution %d writer exited unexpectedly; checking its saved attempt", id)
				retried = true
				continue
			}
			return err
		}
	}
	return nil
}

func runSavedWriter(ctx context.Context, arg string) error {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id < 1 {
		return errors.New("expected a positive execution ID")
	}
	w, err := fromEnv("")
	if err != nil {
		return err
	}
	w.journal, err = w.openJournal(ctx)
	if err != nil {
		return err
	}
	defer w.journal.Close()
	return w.resumeAssignedWriter(ctx, id)
}

func (w *worker) resumeAssignedWriter(ctx context.Context, id int64) error {
	unlock, err := w.lockAttempt(id)
	if err != nil {
		return err
	}
	defer unlock()
	key, err := w.writerAssignment(ctx, id)
	if err != nil || key == nil {
		return err
	}
	q, err := w.readFinalization(ctx, id)
	if err == nil && q == nil {
		q, err = w.readParkedCheckpoint(ctx, id)
	}
	if err != nil {
		return err
	}
	if q == nil {
		var report []byte
		err := w.journal.QueryRowContext(ctx, "SELECT report FROM attempts WHERE lease_hash=? AND settled_at IS NULL", key).Scan(&report)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if len(report) != 0 {
			return w.sendReceipt(ctx, key, id, report)
		}
		log.Printf("execution %d has no confirmed writer checkpoint; retaining the attempt for reconciliation", id)
		return nil
	}
	digest := sha256.Sum256([]byte(q.Job.LeaseToken))
	if !bytes.Equal(digest[:], key) || q.Job.ExecutionID != id ||
		q.Job.Kind != "codex_issue" || q.Job.Spec == nil || (q.Job.Spec.Harness != "" && q.Job.Spec.Harness != "codex") {
		return fmt.Errorf("execution %d does not match its assigned Codex writer", id)
	}
	runningExec := !w.codexAppServer
	if q.SupervisedExec != runningExec {
		log.Printf("execution %d requires its original Codex transport; retaining the writer checkpoint", id)
		return nil
	}
	w.superviseWriters = true
	return w.runLockedAttempt(ctx, q.Job, time.Now(), q)
}
