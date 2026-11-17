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
var errSessionMissing = errors.New("native harness could not find the requested session")

func failedTurnExit(exit error) error {
	if exit != nil {
		return fmt.Errorf("%w: %w", errTurnFailed, exit)
	}
	return errTurnFailed
}

func (w *worker) runJobTurn(ctx context.Context, adapter harnessAdapter, job store.Job, prompt, previous, base string, identity []byte, question *nativeQuestion, plannerOutput string) (string, error) {
	name, version := adapter.identity()
	run := func(prompt, session, handoffFrom string) (string, error) {
		process := w.processLog(ctx, job, name, version)
		process.handoffFrom = handoffFrom
		process.question = question
		if question != nil {
			*question = nativeQuestion{}
		}
		return adapter.runTurn(ctx, process, job.Workspace, prompt, session)
	}
	checkWorkspace := func() error {
		if err := w.confirmRecovery(ctx, job, base, identity); err != nil {
			return err
		}
		if plannerOutput != "" {
			return preparePlannerResume(ctx, job.Workspace, plannerOutput)
		}
		return nil
	}
	runSession := func(input, previous, from string) (string, error) {
		session, err := run(input, previous, from)
		if session == "" {
			return previous, err
		}
		if !errors.Is(err, errTurnInterrupted) || ctx.Err() != nil {
			return session, err
		}
		if err := checkWorkspace(); err != nil {
			return session, err
		}
		_, err = run("The previous process was interrupted. Inspect the existing checkout and finish only the remaining authorized work.\n"+input, session, "")
		return session, err
	}
	session, err := runSession(prompt, previous, "")
	if !errors.Is(err, errSessionMissing) || !job.PlannerHandoff || plannerOutput == "" || !nativeSessionID.MatchString(session) || ctx.Err() != nil {
		return session, err
	}
	if err := checkWorkspace(); err != nil {
		return session, err
	}
	input := fmt.Sprintf("The original read-only planning thread %s is unavailable. Continue execution %d at its frozen commit %s. Repeat discovery in this clean checkout and produce the requested plan using the original scope and discussion below.\n", session, job.ExecutionID, base)
	next, err := runSession(input+prompt, "", session)
	if next == "" {
		return session, err
	}
	return next, err
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
