package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"forgeos/internal/store"
)

type activeWriterTurn struct {
	store.NativeTurn
	Deadline time.Time
}

// Only the process that started the child can collect its exit code.
func (w *worker) waitForWriterExit(ctx context.Context, q *parkedCheckpoint) error {
	if !w.codexAppServer || q.Version != w.codexVersion {
		return errors.New("original Codex App Server version is unavailable")
	}
	key := sha256.Sum256([]byte(q.Job.LeaseToken))
	p, err := scanHarnessProcess(w.journal.QueryRowContext(ctx, readHarnessProcess+
		" WHERE lease_hash=? ORDER BY instance DESC LIMIT 1", key[:]))
	if err != nil {
		return err
	}
	if p.PID <= 0 || p.Harness != "codex" || p.Version != q.Version || p.Instance != q.Active.Instance || p.Turn != q.Active.Turn || p.Session != q.Session {
		return errors.New("latest process does not match the interrupted writer")
	}
	for {
		alive, err := writerTreeAlive(p.PID)
		if err != nil {
			return err
		}
		if !alive {
			if p.Exited != nil {
				return nil
			}
			_, err = w.journal.ExecContext(ctx, "UPDATE harness_processes SET exited_at=CURRENT_TIMESTAMP,exit_code=-1 WHERE instance=? AND exited_at IS NULL", p.Instance)
			return err
		}
		wait(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (w *worker) recoverActiveWriter(ctx context.Context, b *executionResources, q *parkedCheckpoint, base string) error {
	if err := w.waitForWriterExit(ctx, q); err != nil {
		return err
	}
	if err := w.confirmWriterLease(ctx, q, b.recovery); err != nil {
		return err
	}
	if time.Until(q.Active.Deadline) <= 0 {
		b.recovery.waiting, b.recovery.leaveUnsettled = false, false
		return context.DeadlineExceeded
	}
	read, cancel := context.WithTimeout(ctx, 10*time.Second)
	history, err := (codexCLI{bin: w.codex}).readInputHistory(read, q.Job.Workspace, q.Session)
	cancel()
	if err != nil {
		return err
	}
	if !history.interruptedWriter(q) {
		return errors.New("native history does not confirm the interrupted writer as the latest turn")
	}
	if err := w.confirmRecovery(ctx, q.Job, base, q.GitDir); err != nil {
		return err
	}
	trees, err := checkpointTrees(ctx, q.Job.Workspace)
	if err != nil {
		return err
	}
	if q.Trees.Index == "" {
		q.Trees = trees
		if err := w.saveFinalization(ctx, q); err != nil {
			return err
		}
	} else if trees != q.Trees {
		return errors.New("interrupted draft changed after recovery inspection")
	}
	pause, err := w.pollModelPause(ctx, q.Job, store.PausePoll{Step: q.ResourceStep})
	if err != nil {
		return err
	}
	q.Remaining = min(q.Remaining, time.Until(q.Active.Deadline))
	if q.Remaining <= 0 {
		b.recovery.waiting, b.recovery.leaveUnsettled = false, false
		return context.DeadlineExceeded
	}
	if pause == nil {
		// withResource will reclaim the original model reservation at this step.
		b.step = q.ResourceStep - 1
		b.recovery.saved = nil
		return nil
	}
	if pause.Instance != q.Active.Instance || pause.Session != q.Session || pause.Resumed != nil {
		return errors.New("pending model pause does not match interrupted writer")
	}
	b.step = q.ResourceStep
	if err := b.request(ctx, "model", true); err != nil {
		return err
	}
	q.Active = nil
	return w.saveParkedCheckpoint(ctx, *q)
}

func (h inputHistory) interruptedWriter(q *parkedCheckpoint) bool {
	turns := h.Thread.Turns
	return h.matchesThread(q.Job.Workspace, q.Session) && len(turns) > 0 &&
		turns[len(turns)-1].ID == q.Active.Turn && turns[len(turns)-1].Status == "interrupted"
}
