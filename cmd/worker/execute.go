package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func failed(message string, cause error) store.Result {
	code := 1
	var exit *exec.ExitError
	if errors.As(cause, &exit) && exit.ExitCode() > 0 {
		code = exit.ExitCode()
	}
	return store.Result{ExitCode: code, Error: message}
}

func (w *worker) execute(ctx context.Context, job store.Job) store.Result {
	return w.executeAttempt(ctx, job, &checkpointRecovery{})
}

func (w *worker) executeAttempt(ctx context.Context, job store.Job, recovery *checkpointRecovery) store.Result {
	if job.Kind == store.ProjectPreviewKind {
		return w.executeProjectPreview(ctx, job)
	}
	timeout := time.Minute
	if job.Kind == "codex_issue" || job.Kind == "planner_discussion" {
		timeout = 20 * time.Minute
	} else if job.Kind == "environment_check" {
		timeout = 10 * time.Minute
	} else if job.Kind != "" && job.Kind != "git_head" {
		return failed("unsupported task kind", nil)
	}
	leaseCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	repo, err := localrepo.ResolveAssigned(ctx, w.repoRoot, job.RepoPath, job.RepoRelativePath)
	if err != nil {
		return failed("repository validation failed: "+err.Error(), err)
	}
	root, err := filepath.EvalSymlinks(w.workspaceRoot)
	if err != nil || root != w.workspaceRoot {
		return failed("workspace root changed or is unavailable", err)
	}
	if rel, err := filepath.Rel(repo, root); err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return failed("workspace root must be outside the project working tree", nil)
	}
	expected := filepath.Join(root, fmt.Sprintf("execution-%d", job.ExecutionID))
	if rel, err := filepath.Rel(expected, job.Workspace); err != nil || rel != "." || job.ExecutionID <= 0 {
		return failed("unexpected execution workspace", err)
	}
	if recovery.saved != nil {
		dir, err := filepath.EvalSymlinks(expected)
		if err != nil || dir != expected {
			return failed("original execution workspace is unavailable", err)
		}
		common := []string{"rev-parse", "--path-format=absolute", "--git-common-dir"}
		projectGit, err := localrepo.Git(ctx, repo, common...)
		if err != nil {
			return failed("original project metadata is unavailable", err)
		}
		workGit, err := localrepo.Git(ctx, expected, common...)
		if err != nil || !bytes.Equal(workGit, projectGit) {
			return failed("restored workspace does not belong to the assigned project", err)
		}
	} else if _, err := os.Lstat(expected); !errors.Is(err, os.ErrNotExist) {
		return failed("execution workspace already exists or is inaccessible", err)
	}
	if job.Kind == "codex_issue" || job.Kind == "planner_discussion" {
		deadline, _ := ctx.Deadline()
		return w.executeIssue(leaseCtx, repo, expected, job, deadline, recovery)
	}
	if job.Kind == "environment_check" {
		if job.Startup == nil {
			return failed("missing startup configuration", nil)
		}
		if err = os.Mkdir(expected, 0700); err != nil {
			return failed("cannot create startup workspace", err)
		}
		r, err := withResource(ctx, &executionResources{worker: w, job: job}, "ci", func() (*environmentRun, error) {
			r := environmentRun{EnvironmentRequest: store.EnvironmentRequest{Repo: localrepo.RelativePath(w.repoRoot, repo), Commit: job.Startup.Commit, Environment: job.Startup.Environment}, Worker: w.id, Started: time.Now().UTC()}
			var capture *environmentLog
			if job.ReportLiveCILog {
				observe, stop := w.watchCILog(ctx, job, r.Commit)
				defer stop()
				capture = &environmentLog{archive: job.ArchiveStartupLog, observe: observe, run: fmt.Sprintf("ci-run-%d", job.ExecutionID), secrets: outputSecrets(job.LeaseToken), credential: w.credential}
			}
			err := r.record(ctx, repo, expected, capture)
			return &r, err
		})
		var result store.Result
		if err != nil {
			result = failed(err.Error(), err)
		}
		if r != nil && (result.ExitCode == 0 || r.Failure != "") {
			result.EnvironmentRun = (*store.EnvironmentRun)(r)
			result.Commit = r.Commit
		}
		return result
	}
	// git_head needs only the commit. Skipping checkout also avoids running project filters.
	if _, err := localrepo.Git(ctx, repo, "worktree", "add", "--detach", "--no-checkout", expected, "HEAD"); err != nil {
		return failed("git worktree add failed", err)
	}
	out, err := localrepo.Git(ctx, expected, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return failed("git rev-parse HEAD failed", err)
	}
	commit := strings.TrimSpace(string(out))
	if _, err := hex.DecodeString(commit); err != nil || (len(commit) != 40 && len(commit) != 64) {
		return failed("git returned an invalid commit hash", err)
	}
	return store.Result{Commit: commit}
}
