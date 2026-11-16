package main

import (
	"context"
	"io"
	"net/http"
	"strconv"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

type diffStore interface {
	Task(context.Context, int64) (store.Task, error)
	Project(context.Context, int64) (store.Project, error)
}

func taskDiff(data diffStore, root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID, valid := pathID(w, r)
		if !valid {
			return
		}
		query := r.URL.Query()
		executionID, err := strconv.ParseInt(query.Get("execution_id"), 10, 64)
		commit := query.Get("commit")
		if err != nil || executionID <= 0 || !gitCommit.MatchString(commit) {
			problem(w, http.StatusBadRequest, "provide execution_id and the full candidate commit")
			return
		}
		task, err := data.Task(r.Context(), taskID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if task.Kind != "codex_issue" || task.Spec == nil {
			problem(w, http.StatusConflict, "task has no issue candidate")
			return
		}
		var attempt *store.Execution
		for i := range task.Attempts {
			if task.Attempts[i].ID == executionID {
				attempt = &task.Attempts[i]
				break
			}
		}
		if attempt == nil {
			problem(w, http.StatusNotFound, "execution not found for this task")
			return
		}
		base := task.Spec.BaseCommit
		if attempt.Result == nil || attempt.Result.Commit != commit || attempt.Result.BaseCommit != base || !gitCommit.MatchString(base) {
			problem(w, http.StatusConflict, "candidate does not match the recorded execution")
			return
		}
		project, err := data.Project(r.Context(), task.ProjectID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(r.Context(), root, project.Path)
		if err != nil || project.RepositoryID != task.Spec.Issue.RepositoryID {
			problem(w, http.StatusConflict, "registered project repository is unavailable")
			return
		}
		diff, err := readCommitDiff(r.Context(), repo, base, commit)
		if err != nil {
			problem(w, http.StatusConflict, "cannot read the recorded Git objects from the registered repository")
			return
		}
		writeJSON(w, http.StatusOK, diff)
	}
}

type commitDiff struct {
	Diff      string `json:"diff"`
	Truncated bool   `json:"truncated"`
}

const maxDiffBytes = 256 << 10

func readCommitDiff(ctx context.Context, repo, base, commit string) (commitDiff, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := localrepo.GitCommand(ctx, repo, "diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--no-renames", "--submodule=short", "--ignore-submodules=none", base, commit, "--")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return commitDiff{}, err
	}
	if err = cmd.Start(); err != nil {
		return commitDiff{}, err
	}
	output, err := io.ReadAll(io.LimitReader(stdout, maxDiffBytes+1))
	truncated := len(output) > maxDiffBytes
	if truncated || err != nil {
		cancel()
	}
	exit := cmd.Wait()
	switch {
	case err != nil:
		return commitDiff{}, err
	case truncated:
		return commitDiff{string(output[:maxDiffBytes]), true}, nil
	case exit != nil:
		return commitDiff{}, exit
	default:
		return commitDiff{string(output), false}, nil
	}
}
