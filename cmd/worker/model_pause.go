package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"forgeos/internal/store"
)

func (w *worker) pollModelPause(ctx context.Context, job store.Job, input store.PausePoll) (*store.ModelPause, error) {
	input.Lease = job.LeaseToken
	path := fmt.Sprintf("/internal/executions/%d/model-pause", job.ExecutionID)
	for ctx.Err() == nil {
		var pause store.ModelPause
		status, err := w.post(ctx, path, input, &pause)
		if temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil {
			return nil, err
		}
		switch status {
		case http.StatusNoContent:
			return nil, nil
		case http.StatusOK:
			if pause.Instance > 0 && pause.Session != "" {
				return &pause, nil
			}
		case http.StatusGone:
			return nil, store.ErrSourceClosed
		case http.StatusLocked:
			return nil, store.ErrOwnerStop
		}
		return nil, fmt.Errorf("model pause receipt rejected (HTTP %d)", status)
	}
	return nil, ctx.Err()
}

func (w *worker) runImplementationTurn(ctx context.Context, b *executionResources, sequence int, session, base string, identity []byte,
	stopClock func() time.Duration, resumeClock func(time.Duration), run func(context.Context, string, bool) (string, error)) (string, error) {
	resuming := false
	var resumeTrees *store.WorktreeTrees
	for {
		interrupted := false
		next, err := withResource(ctx, b, "model", func() (string, error) {
			if !b.job.ModelPause {
				return run(ctx, session, false)
			}
			if resumeTrees != nil {
				if err := w.verifyQuestionWorkspace(ctx, b.job, base, identity, sequence, resumeTrees); err != nil {
					return session, err
				}
			}
			turn, cancel := context.WithCancelCause(ctx)
			stop := errors.New("Owner requested a model pause")
			done := make(chan struct{})
			go func() {
				defer close(done)
				for turn.Err() == nil {
					wait(turn)
					p, e := w.pollModelPause(turn, b.job, store.PausePoll{Step: b.step})
					if e != nil {
						return
					}
					if p != nil && p.Resumed == nil {
						cancel(stop)
						return
					}
				}
			}()
			got, turnErr := run(turn, session, resuming)
			cancel(nil)
			<-done
			interrupted = context.Cause(turn) == stop && errors.Is(turnErr, context.Canceled)
			return got, turnErr
		})
		if !b.job.ModelPause || ctx.Err() != nil || (err != nil && !interrupted) {
			return next, err
		}
		input := store.PausePoll{Step: b.step}
		p, pollErr := w.pollModelPause(ctx, b.job, input)
		if pollErr != nil {
			return next, pollErr
		}
		if p == nil {
			return next, err
		}
		if next != p.Session || p.Resumed != nil {
			return next, errors.New("pause does not match the active native session")
		}
		if err = w.confirmRecovery(ctx, b.job, base, identity); err != nil {
			return next, err
		}
		trees, err := checkpointTrees(ctx, b.job.Workspace)
		if err != nil {
			return next, err
		}
		remaining := stopClock()
		if remaining <= 0 {
			return next, context.DeadlineExceeded
		}
		input.Session, input.Trees = next, &trees
		for {
			p, err = w.pollModelPause(ctx, b.job, input)
			if err != nil {
				return next, err
			}
			if p == nil || p.Session != next || p.Parked == nil || p.Index != trees.Index || p.Worktree != trees.Worktree {
				return next, errors.New("unconfirmed model pause checkpoint")
			}
			if p.Resumed != nil {
				break
			}
			wait(ctx)
		}
		resumeClock(remaining)
		if !interrupted {
			return next, w.verifyQuestionWorkspace(ctx, b.job, base, identity, sequence, &trees)
		}
		resumeTrees = &trees
		session, resuming = next, true
	}
}
