package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"time"

	"forgeos/internal/store"
)

func (w *worker) managementReviewTurn(ctx context.Context, job store.ManagementJob, prompt string, review *store.ManagementReview) (string, error) {
	if !job.CaptureProcess {
		return readOnlyJSONTurn(ctx, w.codex, reviewSchema, prompt, 12<<10, review)
	}
	version, err := readCodexVersion(w.codex)
	if err != nil {
		return "", err
	}
	node := w.id
	if node == "" {
		node, err = os.Hostname()
		if err != nil {
			return "", err
		}
	}
	// Separate role journals even when a host also runs an ordinary Worker.
	recorder := *w
	recorder.id = "management-" + node
	db, err := recorder.openJournal(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()
	recorder.journal = db
	updates := make(chan struct{}, 1)
	reporting, stop := context.WithCancel(context.WithValue(ctx, processUpdatesKey{}, updates))
	key := sha256.Sum256([]byte(job.Lease))
	target := store.ManagementProcessReport{TaskID: job.TaskID, Revision: job.Revision, Lease: job.Lease, Node: node}
	if job.Share != nil {
		target.ShareID = job.Share.ID
	}
	report := func(call context.Context) error {
		p, err := scanHarnessProcess(db.QueryRowContext(call, readHarnessProcess+" WHERE lease_hash=? ORDER BY instance DESC LIMIT 1", key[:]))
		if err != nil {
			return err
		}
		report := target
		report.Process = p
		status, err := w.post(call, "/internal/management/process", report, nil)
		if err != nil {
			return err
		}
		if status != 204 {
			return fmt.Errorf("management process report failed (HTTP %d)", status)
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
	process := recorder.processLog(reporting, store.Job{LeaseToken: job.Lease, RecordModelOutput: true}, "codex", version)
	session, turnErr := recordedJSONTurn(reporting, w.codex, reviewSchema, prompt, 12<<10, review, process)
	stop()
	<-done
	// Persist the actual exit before allowing its review to advance the plan.
	call, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = report(call)
	return session, errors.Join(turnErr, err)
}
