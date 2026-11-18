package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"time"

	"forgeos/internal/store"
)

func (w *worker) managementReviewTurn(ctx context.Context, job store.ManagementJob, prompt string, review *store.ManagementReview) (string, error) {
	target := store.ManagementProcessReport{TaskID: job.TaskID, Revision: job.Revision, Lease: job.Lease}
	if job.Share != nil {
		target.ShareID = job.Share.ID
	}
	return w.roleJSONTurn(ctx, job.CaptureProcess, target, reviewSchema, prompt, 12<<10, review)
}

func (w *worker) roleJSONTurn(ctx context.Context, capture bool, target store.ManagementProcessReport, contract, prompt string, limit int64, dest any) (string, error) {
	if session, found, err := w.completedRoleTurn(target, dest); found || err != nil {
		return session, err
	}
	if !capture {
		return recordedJSONTurn(ctx, w.codex, contract, prompt, limit, dest, processLog{beforeStart: w.roleStartHook(roleForProcess(target), target.Lease, target.Turn)})
	}
	version, err := readCodexVersion(w.codex)
	if err != nil {
		return "", err
	}
	return w.roleProcessTurn(ctx, target, "codex", version, dest, func(ctx context.Context, process processLog) (string, error) {
		return recordedJSONTurn(ctx, w.codex, contract, prompt, limit, dest, process)
	})
}

func roleForProcess(target store.ManagementProcessReport) string {
	role := "management"
	if target.CurationRole == "curator" {
		role = "curator"
	}
	if target.TestPlanRole == "test" {
		role = "test-agent"
	}
	if target.ReviewerExecution != 0 {
		role = "reviewer"
	}
	return role
}

func (w *worker) roleProcessTurn(ctx context.Context, target store.ManagementProcessReport, harness, version string, dest any, run func(context.Context, processLog) (string, error)) (session string, resultErr error) {
	role := roleForProcess(target)
	db, node, err := w.openRoleJournal(ctx, role)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	// A quote-repair turn cannot overtake the previous turn's exit observation.
	if err = w.replayRoleProcesses(ctx, db, role); err != nil {
		return "", err
	}
	recorder := *w
	recorder.journal = db
	updates := make(chan struct{}, 1)
	reporting, stop := context.WithCancel(context.WithValue(ctx, processUpdatesKey{}, updates))
	key := sha256.Sum256([]byte(target.Lease))
	target.Node = node
	report := func(call context.Context) error {
		// Let the bounded local read close its statement before stopping the reporter.
		p, err := scanHarnessProcess(db.QueryRow(readHarnessProcess+" WHERE lease_hash=? ORDER BY instance DESC LIMIT 1", key[:]))
		if err != nil {
			return err
		}
		report := target
		report.Process = p
		status, err := w.post(call, "/internal/"+role+"/process", report, nil)
		if err != nil {
			return err
		}
		if status != 204 {
			return fmt.Errorf("role process report failed (HTTP %d)", status)
		}
		return nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTicker(15 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-reporting.Done():
				return
			case <-updates:
			case <-timer.C:
			}
			call, cancel := context.WithTimeout(reporting, 5*time.Second)
			_ = report(call)
			cancel()
		}
	}()
	process := recorder.processLog(reporting, store.Job{LeaseToken: target.Lease, RecordModelOutput: true}, harness, version)
	process.beforeStart = func() error { return w.markRoleStarted(db, key[:], target.Turn) }
	session, turnErr := run(reporting, process)
	stop()
	<-done
	p, err := scanHarnessProcess(db.QueryRow(readHarnessProcess+" WHERE lease_hash=? ORDER BY instance DESC LIMIT 1", key[:]))
	if err != nil {
		return session, errors.Join(turnErr, err)
	}
	target.Process = p
	tx, err := db.Begin()
	if err != nil {
		return session, errors.Join(turnErr, err)
	}
	defer tx.Rollback()
	if err = bufferRoleProcess(tx, target); err != nil {
		return session, errors.Join(turnErr, err)
	}
	if turnErr == nil {
		if err = w.bufferCompletedRoleTurn(tx, target, session, dest); err != nil {
			return session, err
		}
	}
	if err = tx.Commit(); err != nil {
		return session, errors.Join(turnErr, err)
	}
	// Delivery failure must not discard a completed model result; its receipt waits for this exit.
	call, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = w.replayRoleProcesses(call, db, role); err != nil {
		log.Printf("%s process exit remains buffered: %v", role, err)
	}
	return session, turnErr
}
