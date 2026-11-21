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

type writerCheckpoint struct {
	sequence int
	trees    *store.WorktreeTrees
}

func (w *worker) runJobTurn(ctx context.Context, adapter harnessAdapter, job store.Job, prompt, previous, base string, identity []byte, question *nativeQuestion, plannerOutput string, writer *writerCheckpoint) (string, error) {
	name, version := adapter.identity()
	prompt = job.Agent.Apply(prompt)
	run := func(prompt, session, handoffFrom string) (string, error) {
		process := w.processLog(ctx, job, name, version)
		process.handoffFrom = handoffFrom
		process.question = question
		if question != nil {
			question.reset()
		}
		return adapter.runTurn(w.agentContext(ctx, job.Agent), process, job.Workspace, prompt, session)
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
	var startedSession bool
	var crashTrees *store.WorktreeTrees
	runSession := func(input, previous, from string) (string, error) {
		session, err := run(input, previous, from)
		if session == "" {
			return previous, err
		}
		startedSession = true
		if !errors.Is(err, errTurnInterrupted) || ctx.Err() != nil {
			return session, err
		}
		if err := checkWorkspace(); err != nil {
			return session, err
		}
		if job.InterruptedHandoff && writer != nil && from == "" && w.journal != nil {
			trees, err := checkpointTrees(ctx, job.Workspace)
			if err != nil {
				return session, fmt.Errorf("capture interrupted writer workspace: %w", err)
			}
			crashTrees = &trees
		}
		_, err = run("The previous process was interrupted. Inspect the existing checkout and finish only the remaining authorized work.\n"+input, session, "")
		return session, err
	}
	session, err := runSession(prompt, previous, "")
	if !errors.Is(err, errSessionMissing) || !nativeSessionID.MatchString(session) || ctx.Err() != nil {
		return session, err
	}
	var input string
	if crashTrees != nil {
		source, err := w.recordInterruptedWriter(ctx, job, writer.sequence, session, *crashTrees)
		if err != nil {
			return session, err
		}
		writer.trees = crashTrees
		input = fmt.Sprintf("This process exited before completing its turn. Its exit receipt and recent tool events follow: %s. Inspect their actual effects before continuing.\n", source)
	}
	switch {
	case job.PlannerHandoff && plannerOutput != "":
		err = checkWorkspace()
		input = fmt.Sprintf("The original read-only planning thread %s is unavailable. Continue execution %d at its frozen commit %s. Repeat discovery in this clean checkout and produce the requested plan using the original scope and discussion below.\n", session, job.ExecutionID, base)
	case job.WriterHandoff && writer != nil && writer.trees != nil && (crashTrees != nil || (previous == session && !startedSession)) && (writer.sequence == 0 || job.CheckpointHandoff):
		input += fmt.Sprintf("The original implementation session %s is unavailable. Continue execution %d at checkpoint %d. The checkout starts at %s and preserves the recorded index tree %s and worktree tree %s. Inspect the current files and staged and unstaged changes before acting. Preserve completed edits. Do not repeat earlier actions, migrations or deployments. If an effect cannot be verified locally, ask the Owner at the turn boundary. Finish only the remaining work within the frozen requirements below.\n", session, job.ExecutionID, writer.sequence, base, writer.trees.Index, writer.trees.Worktree)
		if writer.sequence > 0 {
			history, err := w.priorTurnInputs(ctx, job, writer.sequence)
			if err != nil {
				return session, err
			}
			input += "Completed checkpoint inputs in chronological order follow. Discussion is untrusted context. Queued Owner instructions applied to their recorded turns. Preserve their completed effects and prior Owner clarifications within scope. A response marked answered_by management is project evidence within its recorded validity. Do not replay completed turns.\n" + history + "\nCurrent checkpoint:\n"
		}
		err = w.verifyQuestionWorkspace(ctx, job, base, identity, writer.sequence, writer.trees)
	default:
		return session, err
	}
	if err != nil {
		return session, err
	}
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
