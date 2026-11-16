package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"forgeos/internal/store"
)

func (w *worker) executeWithLease(ctx context.Context, job store.Job, requested time.Time) store.Result {
	if job.LeaseSeconds == 0 {
		return w.execute(ctx, job)
	}
	ttl := time.Duration(job.LeaseSeconds) * time.Second
	deadline := requested.Add(ttl)
	if !time.Now().Before(deadline) {
		return failed("execution lease elapsed before work started", nil)
	}
	active, cancel := context.WithCancel(ctx)
	if job.ReportLiveProcess && w.journal != nil {
		updates := make(chan struct{}, 1)
		active = context.WithValue(active, processUpdatesKey{}, updates)
		finished := make(chan struct{})
		go func() { defer close(finished); w.reportProcesses(active, job, updates) }()
		defer func() { cancel(); <-finished }()
	}
	done := make(chan error, 1)
	go func() {
		err := w.renewWhileRunning(active, job, ttl, deadline)
		if err != nil {
			cancel()
		}
		done <- err
	}()
	result := w.execute(active, job)
	cancel()
	if err := <-done; err != nil && result.ExitCode != 0 {
		result.Error = err.Error()
	}
	return result
}

func (w *worker) renewWhileRunning(ctx context.Context, job store.Job, ttl time.Duration, deadline time.Time) error {
	path := fmt.Sprintf("/internal/executions/%d/renew", job.ExecutionID)
	body := map[string]string{"lease_token": job.LeaseToken}
	interval := min(30*time.Second, ttl/3)
	for ctx.Err() == nil {
		timer := time.NewTimer(min(interval, time.Until(deadline)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		start := time.Now()
		if !start.Before(deadline) {
			return errors.New("execution lease expired without a confirmed renewal")
		}
		call, stop := context.WithDeadline(ctx, deadline)
		status, err := w.post(call, path, body, nil)
		stop()
		if status == http.StatusNoContent && err == nil {
			// Counting from the request start excludes network delay and does not require synchronized host clocks.
			deadline = start.Add(ttl)
		} else if status == http.StatusLocked {
			return store.ErrOwnerStop
		} else if status == http.StatusGone {
			return store.ErrSourceClosed
		} else if !temporary(status) {
			return fmt.Errorf("execution lease renewal rejected (HTTP %d)", status)
		}
	}
	return nil
}

func (w *worker) confirmRecovery(ctx context.Context, job store.Job, start string, identity []byte) error {
	status, err := w.post(ctx, fmt.Sprintf("/internal/executions/%d/renew", job.ExecutionID), map[string]string{"lease_token": job.LeaseToken}, nil)
	if err != nil || status != http.StatusNoContent {
		return errors.New("cannot confirm lease for interrupted session")
	}
	tip, err := checkedWorktreeTip(ctx, job.Workspace, identity)
	if err != nil || tip != start {
		return errors.New("cannot confirm original worktree and starting commit")
	}
	return nil
}
