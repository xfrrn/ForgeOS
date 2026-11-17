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
	"time"

	"forgeos/internal/store"
)

// Written after the native turn exits, before exposing its checkpoint for Owner input.
type parkedCheckpoint struct {
	Job                                  store.Job
	Sequence                             int
	Session, Question, Evidence, Version string
	GitDir                               []byte
	Trees                                store.WorktreeTrees
	Remaining                            time.Duration
	ReferenceDecisionID                  int64
	ResourceStep                         int64
	Planner                              *pausedPlanner `json:",omitempty"`
	Writer                               *pausedWriter  `json:",omitempty"`
}

type pausedPlanner struct {
	Comments []store.CommentInput
	Finished *store.Plan
}

type pausedWriter struct {
	Batch      store.CheckpointPage
	OwnerReply json.RawMessage
	Finished   bool
	Asked      bool
}

type checkpointRecovery struct {
	saved                   *parkedCheckpoint
	waiting, leaveUnsettled bool
}

func (w *worker) nativeVersion(job store.Job) string {
	if job.Spec.Harness == "claude" {
		return w.claude.version
	}
	return w.codexVersion
}

func (w *worker) resumeOwnerQuestion(ctx context.Context, job store.Job, saved parkedCheckpoint, start string, recovery *checkpointRecovery, resumeBudget func(time.Duration)) (json.RawMessage, error) {
	q := nativeQuestion{ReferenceDecisionID: saved.ReferenceDecisionID, Question: saved.Question, Evidence: saved.Evidence, requested: true}
	answer, err := w.waitForOwner(ctx, job, saved.Sequence, saved.Session, q, &saved.Trees)
	if err != nil {
		if errors.Is(err, store.ErrQuestionClaimed) {
			recovery.leaveUnsettled = true
		}
		if errors.Is(err, store.ErrConflict) {
			// Cancellation needs a stop receipt. A stale or already advanced turn must not settle a competing attempt.
			status, _ := w.post(ctx, fmt.Sprintf("/internal/executions/%d/renew", job.ExecutionID), map[string]string{"lease_token": job.LeaseToken}, nil)
			recovery.leaveUnsettled = status != http.StatusLocked && status != http.StatusGone
		}
		return nil, err
	}
	recovery.leaveUnsettled = false
	recovery.waiting = false
	resumeBudget(saved.Remaining)
	if saved.Version != w.nativeVersion(job) {
		return nil, errors.New("native harness version changed since checkpoint")
	}
	if err = w.verifyQuestionWorkspace(ctx, job, start, saved.GitDir, saved.Sequence, &saved.Trees); err != nil {
		return nil, err
	}
	return answer, w.removeParkedCheckpoint(ctx, job)
}

func (w *worker) removeParkedCheckpoint(ctx context.Context, job store.Job) error {
	key := sha256.Sum256([]byte(job.LeaseToken))
	result, err := w.journal.ExecContext(ctx, `DELETE FROM parked_questions WHERE lease_hash=?`, key[:])
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return errors.New("local checkpoint disappeared before resume")
	}
	return nil
}

func (w *worker) verifyQuestionWorkspace(ctx context.Context, job store.Job, start string, dir []byte, seq int, trees *store.WorktreeTrees) error {
	if err := w.confirmRecovery(ctx, job, start, dir); err != nil {
		return err
	}
	if trees == nil {
		return nil
	}
	current, err := checkpointTrees(ctx, job.Workspace)
	if err != nil {
		return fmt.Errorf("verify worktree at checkpoint %d: %w", seq, err)
	}
	if current != *trees {
		return fmt.Errorf("worktree or index changed while waiting for Owner at checkpoint %d", seq)
	}
	return nil
}

func (w *worker) saveParkedCheckpoint(ctx context.Context, q parkedCheckpoint) error {
	raw, err := json.Marshal(q)
	if err != nil {
		return err
	}
	key := sha256.Sum256([]byte(q.Job.LeaseToken))
	_, err = w.journal.ExecContext(ctx, `INSERT INTO parked_questions(lease_hash,checkpoint) VALUES(?,?)`, key[:], raw)
	return err
}

func (w *worker) readParkedCheckpoint(ctx context.Context, id int64) (*parkedCheckpoint, error) {
	var raw, key []byte
	var workspace string
	var taskID int64
	err := w.journal.QueryRowContext(ctx, `SELECT p.checkpoint,a.lease_hash,a.workspace,a.task_id FROM parked_questions p JOIN attempts a USING(lease_hash)
		WHERE a.execution_id=? AND a.buffered_at IS NULL AND a.settled_at IS NULL`, id).Scan(&raw, &key, &workspace, &taskID)
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
	if !bytes.Equal(digest[:], key) || id != q.Job.ExecutionID || taskID != q.Job.TaskID || workspace != q.Job.Workspace || q.Job.Spec == nil || q.Job.LeaseSeconds <= 0 ||
		q.Job.Kind != "codex_issue" || q.Job.Spec.VerifyCommit != "" ||
		q.Remaining <= 0 || q.Remaining > 20*time.Minute || q.Session == "" || q.Sequence < 0 || len(q.GitDir) == 0 ||
		q.ResourceStep < 0 || q.Job.ManageResources && q.ResourceStep == 0 {
		return nil, fmt.Errorf("invalid stored checkpoint for execution %d", id)
	}
	if q.Planner != nil {
		if q.Writer != nil || q.Job.Spec.Mode != "plan" || !q.Job.RecoverPlannerPause || !q.Job.ModelPause || !q.Job.ManageResources || q.Sequence != 0 ||
			(q.Planner.Finished != nil && q.Planner.Finished.Validate() != nil) {
			return nil, fmt.Errorf("invalid stored planner checkpoint for execution %d", id)
		}
	} else if q.Job.Spec.Mode == "plan" || !q.Job.ResumeQuestions {
		return nil, fmt.Errorf("invalid stored question checkpoint for execution %d", id)
	} else if w := q.Writer; w != nil && (!q.Job.RecoverWriterPause || !q.Job.ModelPause || !q.Job.ManageResources || w.Asked && !w.Finished) {
		return nil, fmt.Errorf("invalid stored implementation checkpoint for execution %d", id)
	}
	return &q, nil
}

func (w *worker) parkedExecutions(ctx context.Context) ([]int64, error) {
	rows, err := w.journal.QueryContext(ctx, `SELECT a.execution_id FROM parked_questions JOIN attempts a USING(lease_hash)
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
