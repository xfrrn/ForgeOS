package main

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os/exec"

	"forgeos/internal/store"
)

type processLog struct {
	db      *sql.DB
	lease   [32]byte
	harness string
	version string
	id      int64
}

func (w *worker) processLog(job store.Job, harness, version string) processLog {
	return processLog{db: w.journal, lease: sha256.Sum256([]byte(job.LeaseToken)), harness: harness, version: version}
}

func (p *processLog) start(cmd *exec.Cmd, stream io.Closer) error {
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
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("record harness process: %w", err)
	}
	return nil
}

func (p *processLog) session(session string) error {
	if p.db == nil {
		return nil
	}
	_, err := p.db.Exec("UPDATE harness_processes SET session_id=? WHERE instance=?", session, p.id)
	return err
}

func (p *processLog) wait(cmd *exec.Cmd) (exitErr, journalErr error) {
	exitErr = cmd.Wait()
	if p.db != nil {
		_, journalErr = p.db.Exec("UPDATE harness_processes SET exited_at=CURRENT_TIMESTAMP,exit_code=? WHERE instance=?", cmd.ProcessState.ExitCode(), p.id)
	}
	return exitErr, journalErr
}
