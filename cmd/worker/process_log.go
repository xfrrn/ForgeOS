package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os/exec"

	"forgeos/internal/store"
)

type processLog struct {
	handoffFrom string
	db          *sql.DB
	lease       [32]byte
	harness     string
	version     string
	updates     chan struct{}
	id          int64
	tools       *toolRecorder
	question    *nativeQuestion
}

func (w *worker) processLog(ctx context.Context, job store.Job, harness, version string) processLog {
	updates, _ := ctx.Value(processUpdatesKey{}).(chan struct{})
	var recorder *toolRecorder
	if job.RecordToolActivity && w.journal != nil {
		recorder = &toolRecorder{pending: make(map[string]toolCall)}
	}
	return processLog{tools: recorder, updates: updates, db: w.journal, lease: sha256.Sum256([]byte(job.LeaseToken)), harness: harness, version: version}
}

func (p *processLog) notify() {
	select {
	case p.updates <- struct{}{}:
	default:
	}
}
func (p *processLog) start(cmd *exec.Cmd, stream io.Closer) error {
	configureHarnessCancellation(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start harness process: %w", err)
	}
	if p.db == nil {
		return nil
	}
	record, err := p.db.Exec("INSERT INTO harness_processes(lease_hash,harness,version,pid) VALUES(?,?,?,?)",
		p.lease[:], p.harness, p.version, cmd.Process.Pid)
	if err == nil {
		p.id, err = record.LastInsertId()
	}
	if err != nil {
		stream.Close()
		_ = cmd.Cancel()
		_ = cmd.Wait()
		return fmt.Errorf("record harness process: %w", err)
	}
	p.notify()
	return nil
}

func (p *processLog) session(session string) error {
	if p.db == nil {
		return nil
	}
	if p.handoffFrom != "" {
		if session == p.handoffFrom {
			return fmt.Errorf("handoff returned the missing session")
		}
		if _, err := p.db.Exec("INSERT INTO harness_session_handoffs(instance,previous_session) VALUES(?,?) ON CONFLICT(instance) DO NOTHING", p.id, p.handoffFrom); err != nil {
			return err
		}
	}
	_, err := p.db.Exec("UPDATE harness_processes SET session_id=? WHERE instance=?", session, p.id)
	if err == nil {
		p.notify()
	}
	return err
}

func (p *processLog) wait(cmd *exec.Cmd) (exitErr, journalErr error) {
	exitErr = cmd.Wait()
	if p.db != nil {
		_, journalErr = p.db.Exec("UPDATE harness_processes SET exited_at=CURRENT_TIMESTAMP,exit_code=? WHERE instance=?", cmd.ProcessState.ExitCode(), p.id)
		if journalErr == nil {
			p.notify()
		}
	}
	return exitErr, journalErr
}
