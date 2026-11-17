package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

// Each call starts one process. A non-empty session resumes the existing conversation.
type harnessAdapter interface {
	identity() (name, version string)
	runTurn(ctx context.Context, process processLog, workspace, prompt, session string) (string, error)
}

var errTurnInterrupted = errors.New("harness exited before the turn result")
var errTurnFailed = errors.New("harness reported a failed turn")

func failedTurnExit(exit error) error {
	if exit != nil {
		return fmt.Errorf("%w: %w", errTurnFailed, exit)
	}
	return errTurnFailed
}

func (w *worker) runJobTurn(ctx context.Context, adapter harnessAdapter, job store.Job, prompt, previous, base string, identity []byte, question *nativeQuestion, plannerOutput string) (string, error) {
	name, version := adapter.identity()
	run := func(prompt, session string) (string, error) {
		process := w.processLog(ctx, job, name, version)
		process.question = question
		if question != nil {
			*question = nativeQuestion{}
		}
		return adapter.runTurn(ctx, process, job.Workspace, prompt, session)
	}
	session, err := run(prompt, previous)
	if session == "" {
		return previous, err
	}
	if !errors.Is(err, errTurnInterrupted) || ctx.Err() != nil {
		return session, err
	}
	if err := w.confirmRecovery(ctx, job, base, identity); err != nil {
		return session, err
	}
	if plannerOutput != "" {
		if err := preparePlannerResume(ctx, job.Workspace, plannerOutput); err != nil {
			return session, err
		}
	}
	_, err = run("The previous process was interrupted. Inspect the existing checkout and finish only the remaining authorized work.\n"+prompt, session)
	return session, err
}

func preparePlannerResume(ctx context.Context, workspace, output string) error {
	status, err := localrepo.Git(ctx, workspace, "status", "--porcelain", "--untracked-files=all")
	if err != nil || len(status) != 0 {
		return errors.New("cannot resume in a changed planner checkout")
	}
	if err := os.Remove(output); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot clear incomplete planner output")
	}
	return nil
}
