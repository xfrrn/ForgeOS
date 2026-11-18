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
	turnInputs    *turnInputClient
	beforeStart   func() error
	roleNative    *roleNativeTurn
	handoffFrom   string
	db            *sql.DB
	lease         [32]byte
	harness       string
	version       string
	updates       chan struct{}
	id            int64
	tools         *toolRecorder
	question      *nativeQuestion
	output        *modelOutput
	toolOutput    *modelOutput
	toolStream    bool
	recordAppTurn bool
}

func (w *worker) processLog(ctx context.Context, job store.Job, harness, version string) processLog {
	updates, _ := ctx.Value(processUpdatesKey{}).(chan struct{})
	var recorder *toolRecorder
	if job.RecordToolActivity && w.journal != nil {
		recorder = &toolRecorder{pending: make(map[string]toolCall)}
	}
	p := processLog{tools: recorder, updates: updates, db: w.journal, lease: sha256.Sum256([]byte(job.LeaseToken)), harness: harness, version: version, recordAppTurn: job.RecordAppTurn}
	if job.SupportsTurnInput && w.codexAppServer && harness == "codex" && w.journal != nil {
		p.turnInputs = &turnInputClient{worker: w, execution: job.ExecutionID, lease: job.LeaseToken}
	}
	if job.RecordModelOutput && w.journal != nil {
		p.output = &modelOutput{secrets: outputSecrets(job.LeaseToken), credential: w.credential}
	}
	if job.RecordToolOutput && recorder != nil {
		p.toolOutput = &modelOutput{secrets: outputSecrets(job.LeaseToken), credential: w.credential}
		p.toolStream = job.RecordToolStream
	}
	return p
}

func (p *processLog) notify() {
	select {
	case p.updates <- struct{}{}:
	default:
	}
}
func (p *processLog) start(cmd *exec.Cmd, stream io.Closer) error {
	if p.beforeStart != nil {
		if err := p.beforeStart(); err != nil {
			stream.Close()
			if output, ok := cmd.Stdout.(io.Closer); ok {
				output.Close()
			}
			return err
		}
	}
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
	if err == nil && p.roleNative != nil {
		_, err = p.db.Exec("INSERT INTO role_native_attempts(instance,resumed_from) VALUES(?,?)", p.id, p.roleNative.Instance)
	}
	if err == nil && p.roleNative != nil {
		var changed sql.Result
		changed, err = p.db.Exec("UPDATE role_native_turns SET instance=? WHERE lease_hash=? AND turn=? AND instance IS NULL", p.id, p.lease[:], p.roleNative.Turn)
		if err == nil {
			var n int64
			n, err = changed.RowsAffected()
			if err == nil && n != 1 {
				err = fmt.Errorf("native role turn lost its startup slot")
			}
		}
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

func (p *processLog) appTurn(turn string) error {
	if !p.recordAppTurn || p.db == nil {
		return nil
	}
	if !store.ValidAppTurnID(turn) {
		return fmt.Errorf("invalid App Server turn identifier")
	}
	var saved string
	err := p.db.QueryRow("INSERT INTO harness_app_turns(instance,turn_id) VALUES(?,?) ON CONFLICT(instance) DO UPDATE SET turn_id=excluded.turn_id WHERE turn_id=excluded.turn_id RETURNING turn_id", p.id, turn).Scan(&saved)
	if err == nil {
		p.notify()
	}
	return err
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
