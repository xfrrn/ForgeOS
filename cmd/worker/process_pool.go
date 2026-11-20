package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"forgeos/internal/store"
	"io"
	"log"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

const poolBurstDelay = 15 * time.Second

type processPool struct {
	limits  atomic.Pointer[store.PoolLimits]
	running atomic.Int32
}

// Each queue iteration may claim at most one eligible job. No process starts for an empty claim.
func poolCapacity(policy store.PoolLimits, occupied int, now time.Time, saturated *time.Time) int {
	if occupied < policy.Normal {
		*saturated = time.Time{}
		return policy.Normal
	}
	if saturated.IsZero() {
		*saturated = now
	}
	if now.Sub(*saturated) >= poolBurstDelay {
		return policy.Maximum
	}
	return policy.Normal
}

type poolWork struct {
	Job       store.Job
	Requested time.Time
	Follow    int64
	Saved     *parkedCheckpoint
	Journal   string
}

func (task poolWork) run(ctx context.Context, w *worker) error {
	if task.Follow != 0 {
		return w.followWriter(ctx, task.Follow)
	}
	err := w.runAttempt(ctx, task.Job, task.Requested, task.Saved)
	if errors.Is(err, errWriterDelegated) {
		return w.followWriter(ctx, task.Job.ExecutionID)
	}
	return err
}

func (w *worker) runPoolProcess(ctx context.Context, task poolWork) error {
	var seq int
	var name string
	if err := w.journal.QueryRowContext(ctx, "PRAGMA database_list").Scan(&seq, &name, &task.Journal); err != nil {
		return err
	}
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "pool-work")
	cmd.Stderr = os.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	// The pipe stays open while the supervisor owns the execution. EOF also handles parent crashes.
	cmd.Cancel = input.Close
	cmd.WaitDelay = 15 * time.Second
	if err = cmd.Start(); err != nil {
		return err
	}
	w.pool.running.Add(1)
	defer w.pool.running.Add(-1)
	log.Printf("started execution worker pid=%d execution=%d", cmd.Process.Pid, max(task.Follow, task.Job.ExecutionID))
	if err = json.NewEncoder(input).Encode(task); err != nil {
		input.Close()
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}
func acceptPoolWork(parent context.Context, input io.Reader) error {
	var task poolWork
	if err := json.NewDecoder(input).Decode(&task); err != nil {
		return err
	}
	if task.Follow <= 0 && (task.Job.ExecutionID <= 0 || task.Job.LeaseToken == "") {
		return errors.New("missing worker assignment")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() { _, _ = io.Copy(io.Discard, input); cancel() }()
	w, err := fromEnv("")
	if err != nil {
		return err
	}
	w.journal, err = openExistingJournal(task.Journal, true)
	if err != nil {
		return fmt.Errorf("open execution journal: %w", err)
	}
	defer w.journal.Close()
	return task.run(ctx, w)
}
