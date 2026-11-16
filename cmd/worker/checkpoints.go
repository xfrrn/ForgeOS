package main

import (
	"context"
	"fmt"
	"forgeos/internal/store"
	"net/http"
)

func (w *worker) recordCICandidate(ctx context.Context, job store.Job, sha, session string) error {
	for ctx.Err() == nil {
		status, err := w.post(ctx, fmt.Sprintf("/internal/executions/%d/ci-candidate", job.ExecutionID), map[string]string{"lease_token": job.LeaseToken, "commit": sha, "session_id": session}, nil)
		if status == http.StatusNoContent && err == nil {
			return nil
		}
		if !temporary(status) {
			return fmt.Errorf("CI candidate checkpoint rejected (HTTP %d)", status)
		}
		wait(ctx)
	}
	return ctx.Err()
}

func (w *worker) checkpoint(ctx context.Context, job store.Job, seq int, session string) ([]store.CommentInput, error) {
	if !job.SupportsCheckpoints {
		return nil, nil
	}
	var comments []store.CommentInput
	var after int64
	for ctx.Err() == nil {
		var page store.CheckpointPage
		status, err := w.post(ctx, fmt.Sprintf("/internal/executions/%d/checkpoints", job.ExecutionID), map[string]any{
			"lease_token": job.LeaseToken, "sequence": seq, "after": after, "session_id": session}, &page)
		if temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil {
			return nil, err
		}
		if session != "" && status == http.StatusNoContent {
			return nil, nil
		}
		if status != http.StatusOK || page.ExecutionID != job.ExecutionID || page.Sequence != seq {
			return nil, fmt.Errorf("checkpoint %d not acknowledged (HTTP %d)", seq, status)
		}
		comments = append(comments, page.Comments...)
		if !page.More {
			return comments, nil
		}
		if len(page.Comments) == 0 || page.Comments[len(page.Comments)-1].EventID <= after {
			return nil, fmt.Errorf("checkpoint %d cursor did not advance", seq)
		}
		after = page.Comments[len(page.Comments)-1].EventID
	}
	return nil, ctx.Err()
}
