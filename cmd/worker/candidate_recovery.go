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

// Save the completed draft before changing its index or creating a candidate SHA.
func (w *worker) prepareFinalization(ctx context.Context, q *parkedCheckpoint, start string, result store.Result) error {
	if err := captureCompletedTree(ctx, q, start); err != nil {
		return err
	}
	return w.promoteCompletedTurn(ctx, q, start, result)
}

func captureCompletedTree(ctx context.Context, q *parkedCheckpoint, start string) error {
	workspace := q.Job.Workspace
	tip, err := checkedWorktreeTip(ctx, workspace, q.GitDir)
	if err != nil {
		return err
	}
	if _, err := localrepo.Git(ctx, workspace, "merge-base", "--is-ancestor", start, tip); err != nil {
		return errors.New("worktree history has diverged from the execution's starting commit")
	}
	q.Trees, err = checkpointTrees(ctx, workspace)
	q.CompletedTip = tip
	return err
}

func (w *worker) promoteCompletedTurn(ctx context.Context, q *parkedCheckpoint, start string, result store.Result) error {
	if err := validateCandidateTree(ctx, q.Job.Workspace, q.Job.Spec.BaseCommit, start, q.Trees.Worktree); err != nil {
		return err
	}
	q.Finalization = &candidateFinalization{Tip: q.CompletedTip, Result: result}
	q.CompletedTip = ""
	return w.saveFinalization(ctx, q)
}

// The server freezes each sequence. Replay both requests before deciding whether the draft is final.
func (w *worker) advanceCompletedTurn(ctx context.Context, q *parkedCheckpoint, recovery *checkpointRecovery) (store.CheckpointPage, error) {
	var page store.CheckpointPage
	recovery.waiting, recovery.leaveUnsettled = true, true
	defer func() {
		if cause := context.Cause(ctx); errors.Is(cause, store.ErrOwnerStop) || errors.Is(cause, store.ErrSourceClosed) || errors.Is(cause, context.DeadlineExceeded) {
			recovery.waiting, recovery.leaveUnsettled = false, false
		}
	}()
	status, err := w.post(ctx, fmt.Sprintf("/internal/executions/%d/renew", q.Job.ExecutionID), map[string]string{"lease_token": q.Job.LeaseToken}, nil)
	if err != nil || status != http.StatusNoContent {
		if status == http.StatusLocked || status == http.StatusGone {
			recovery.waiting, recovery.leaveUnsettled = false, false
		}
		return page, errors.New("cannot confirm lease for completed writer turn")
	}
	if _, err := w.checkpoint(ctx, q.Job, q.Sequence, q.Session); err != nil {
		return page, err
	}
	page, err = w.checkpoint(ctx, q.Job, q.Sequence+1, "")
	if err != nil {
		return page, err
	}
	tip, err := checkedWorktreeTip(ctx, q.Job.Workspace, q.GitDir)
	if err != nil || tip != q.CompletedTip {
		return page, errors.New("completed writer worktree HEAD changed")
	}
	trees, err := checkpointTrees(ctx, q.Job.Workspace)
	if err != nil || trees != q.Trees {
		return page, errors.New("completed writer draft or index changed")
	}
	if len(page.Comments) == 0 && len(page.OwnerInstructions) == 0 {
		recovery.leaveUnsettled = false
		return page, nil
	}
	if q.Version != w.nativeVersion(q.Job) {
		return page, errors.New("native harness version changed before the next writer turn")
	}
	// Once another turn can start, this receipt must never authorize replay of its input.
	key := sha256.Sum256([]byte(q.Job.LeaseToken))
	_, err = w.journal.ExecContext(ctx, "DELETE FROM candidate_finalizations WHERE lease_hash=?", key[:])
	if err == nil {
		recovery.waiting, recovery.leaveUnsettled = false, false
	}
	return page, err
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
		q.ResourceStep < 0 || q.Planner != nil || q.Writer != nil {
		return nil, fmt.Errorf("invalid saved candidate for execution %d", id)
	}
	if q.CompletedTip != "" {
		if q.Finalization != nil || !q.Job.SupportsCheckpoints || q.Sequence < 0 || q.Session == "" || q.Trees.Index == "" || q.Trees.Worktree == "" {
			return nil, fmt.Errorf("invalid completed writer turn for execution %d", id)
		}
	} else if f := q.Finalization; f == nil || f.Tip == "" || f.Result.BaseCommit != q.Job.Spec.BaseCommit || f.Result.SessionID != q.Session ||
		f.Result.Commit == "" && (f.Validated || q.Trees.Index == "" || q.Trees.Worktree == "") {
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
	trees, err := checkpointTrees(ctx, job.Workspace)
	if err != nil {
		return fail(err)
	}
	if q.Trees.Worktree != "" && (trees.Worktree != q.Trees.Worktree || (trees.Index != q.Trees.Index && trees.Index != q.Trees.Worktree)) {
		return fail(errors.New("completed draft or index changed before candidate validation"))
	}
	if saved.Result.Commit == "" {
		saved.Result.Commit, err = createCandidateCommit(ctx, job.Workspace, q.Trees.Worktree, job.Spec.BaseCommit, job.Spec.Issue.Number)
		if err != nil {
			return fail(err)
		}
		if err := w.saveFinalization(ctx, q); err != nil {
			return fail(err)
		}
	}
	parents, err := localrepo.Git(ctx, job.Workspace, "rev-list", "-1", "--parents", saved.Result.Commit, "--")
	if err != nil || strings.TrimSpace(string(parents)) != saved.Result.Commit+" "+job.Spec.BaseCommit {
		return fail(errors.New("saved candidate has an unexpected parent"))
	}
	tree, err := localrepo.Git(ctx, job.Workspace, "rev-parse", saved.Result.Commit+"^{tree}")
	if err != nil {
		return fail(err)
	}
	if trees.Worktree != strings.TrimSpace(string(tree)) {
		return fail(errors.New("candidate index or worktree changed after checkpoint"))
	}
	if trees.Index != trees.Worktree {
		// Older candidate receipts were saved after staging. Only a recorded original index may advance.
		if tip != saved.Tip || q.Trees.Index != trees.Index || q.Trees.Worktree != trees.Worktree {
			return fail(errors.New("candidate index changed after checkpoint"))
		}
		if _, err := localrepo.Git(ctx, job.Workspace, "read-tree", trees.Worktree); err != nil {
			return fail(err)
		}
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
