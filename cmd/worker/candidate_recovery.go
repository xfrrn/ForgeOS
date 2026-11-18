package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

// Persist the candidate before attaching HEAD; recovery never invents another SHA.
type candidateFinalization struct {
	Tip       string
	Result    store.Result
	Validated bool
}

func (w *worker) lockAttempt(id int64) (func(), error) {
	var seq int
	var name, path string
	if err := w.journal.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("execution recovery requires a file journal")
	}
	path += fmt.Sprintf(".execution-%d.lock", id)
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return nil, errors.New("execution lock must be a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockRoleFile(f); err != nil {
		f.Close()
		return nil, errors.New("another Daemon owns this execution, or its journal cannot be locked")
	}
	return func() { f.Close() }, nil
}

func (w *worker) saveFinalization(ctx context.Context, q *parkedCheckpoint) error {
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	key := sha256.Sum256([]byte(q.Job.LeaseToken))
	_, err = w.journal.ExecContext(ctx, `INSERT INTO candidate_finalizations(lease_hash,checkpoint) VALUES(?,?)
 ON CONFLICT(lease_hash) DO UPDATE SET checkpoint=excluded.checkpoint`, key[:], raw)
	return err
}

func (w *worker) finalizingExecutions(ctx context.Context) ([]int64, error) {
	rows, err := w.journal.QueryContext(ctx, `SELECT a.execution_id FROM candidate_finalizations f JOIN attempts a USING(lease_hash)
 WHERE a.buffered_at IS NULL AND a.settled_at IS NULL ORDER BY a.execution_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (w *worker) readFinalization(ctx context.Context, id int64) (*parkedCheckpoint, error) {
	var raw, key []byte
	var task int64
	var workspace string
	err := w.journal.QueryRowContext(ctx, `SELECT f.checkpoint,a.lease_hash,a.task_id,a.workspace FROM candidate_finalizations f JOIN attempts a USING(lease_hash)
 WHERE a.execution_id=? AND a.buffered_at IS NULL AND a.settled_at IS NULL`, id).Scan(&raw, &key, &task, &workspace)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var q parkedCheckpoint
	if err = json.Unmarshal(raw, &q); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(q.Job.LeaseToken))
	if !bytes.Equal(digest[:], key) || q.Job.ExecutionID != id || q.Job.TaskID != task || q.Job.Workspace != workspace ||
		q.Job.Kind != "codex_issue" || q.Job.Spec == nil || q.Job.Spec.Mode == "plan" || q.Job.Spec.VerifyCommit != "" ||
		q.Job.LeaseSeconds <= 0 || q.Remaining <= 0 || q.Remaining > 20*time.Minute || len(q.GitDir) == 0 ||
		q.ResourceStep < 0 || q.Planner != nil || q.Writer != nil || q.Finalization == nil || q.Finalization.Tip == "" ||
		q.Finalization.Result.Commit == "" || q.Finalization.Result.BaseCommit != q.Job.Spec.BaseCommit || q.Finalization.Result.SessionID != q.Session {
		return nil, fmt.Errorf("invalid saved candidate for execution %d", id)
	}
	return &q, nil
}

func (w *worker) finishCandidate(parent context.Context, repo string, q *parkedCheckpoint, recovery *checkpointRecovery) store.Result {
	ctx, cancel := context.WithTimeout(parent, q.Remaining)
	defer cancel()
	recovery.leaveUnsettled = true
	job, saved := q.Job, q.Finalization
	fail := func(err error) store.Result {
		cause := context.Cause(parent)
		if errors.Is(cause, store.ErrOwnerStop) || errors.Is(cause, store.ErrSourceClosed) || errors.Is(err, context.DeadlineExceeded) {
			recovery.waiting, recovery.leaveUnsettled = false, false
		}
		return failed("candidate recovery: "+err.Error(), err)
	}
	status, err := w.post(ctx, fmt.Sprintf("/internal/executions/%d/renew", job.ExecutionID), map[string]string{"lease_token": job.LeaseToken}, nil)
	if err != nil || status != http.StatusNoContent {
		if status == http.StatusLocked || status == http.StatusGone {
			recovery.waiting, recovery.leaveUnsettled = false, false
		}
		return fail(errors.New("cannot confirm candidate lease"))
	}
	tip, err := checkedWorktreeTip(ctx, job.Workspace, q.GitDir)
	if err != nil {
		return fail(err)
	}
	if tip != saved.Tip && tip != saved.Result.Commit {
		return fail(errors.New("candidate worktree HEAD changed"))
	}
	parents, err := localrepo.Git(ctx, job.Workspace, "rev-list", "-1", "--parents", saved.Result.Commit, "--")
	if err != nil || strings.TrimSpace(string(parents)) != saved.Result.Commit+" "+job.Spec.BaseCommit {
		return fail(errors.New("saved candidate has an unexpected parent"))
	}
	tree, err := localrepo.Git(ctx, job.Workspace, "rev-parse", saved.Result.Commit+"^{tree}")
	if err != nil {
		return fail(err)
	}
	trees, err := checkpointTrees(ctx, job.Workspace)
	if err != nil || trees.Index != strings.TrimSpace(string(tree)) || trees.Worktree != trees.Index {
		return fail(errors.New("candidate index or worktree changed after checkpoint"))
	}
	if tip != saved.Result.Commit {
		if _, err = localrepo.Git(ctx, job.Workspace, "update-ref", "--no-deref", "HEAD", saved.Result.Commit, tip); err != nil {
			return fail(err)
		}
	}
	b := executionResources{worker: w, job: job, step: q.ResourceStep}
	if saved.Validated {
		if job.ManageResources {
			b.step++
			if err = b.request(ctx, "ci", true); err != nil {
				return fail(err)
			}
		}
	} else {
		if job.ReportCICandidate {
			if err = w.recordCICandidate(ctx, job, saved.Result.Commit, q.Session); err != nil {
				return fail(err)
			}
		}
		if job.ManageResources {
			b.step++
			if err = b.request(ctx, "ci", false); err != nil {
				return fail(err)
			}
		}
		report, checkErr := w.validateIssueCandidate(ctx, job, repo, job.Workspace, saved.Result.Commit, job.Spec)
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		if report.Commit != "" {
			saved.Result.Validation = &report
		}
		if checkErr == nil && report.ExitCode != 0 {
			checkErr = errors.New("local validation failed")
		}
		if checkErr != nil {
			saved.Result.ExitCode, saved.Result.Error = failed("", checkErr).ExitCode, checkErr.Error()
		}
		saved.Validated = true
		if err = w.saveFinalization(context.WithoutCancel(ctx), q); err != nil {
			return fail(err)
		}
		if job.ManageResources {
			if err = b.request(ctx, "ci", true); err != nil {
				return fail(err)
			}
		}
	}
	recovery.waiting, recovery.leaveUnsettled = false, false
	return saved.Result
}
