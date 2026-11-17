package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"forgeos/internal/store"
	"net/http"
	"strings"
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

// Only the adapter's final assistant message may ask for a business decision.
type nativeQuestion struct {
	Question  string `json:"forgeos_question"`
	Evidence  string `json:"evidence"`
	requested bool
}

func (q *nativeQuestion) read(message string) {
	*q = nativeQuestion{}
	message = strings.TrimSpace(message)
	if strings.HasPrefix(message, "```json") && strings.HasSuffix(message, "```") {
		message = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(message, "```json"), "```"))
	}
	var fields map[string]json.RawMessage
	if !strings.HasPrefix(message, "{") || json.Unmarshal([]byte(message), &fields) != nil || fields["forgeos_question"] == nil {
		return
	}
	q.requested = true
	if json.Unmarshal(fields["forgeos_question"], &q.Question) != nil || json.Unmarshal(fields["evidence"], &q.Evidence) != nil {
		q.Question = ""
	}
}

func (w *worker) waitForOwner(ctx context.Context, job store.Job, seq int, session string, q nativeQuestion, trees *store.WorktreeTrees) (json.RawMessage, error) {
	if strings.TrimSpace(q.Question) == "" || strings.TrimSpace(q.Evidence) == "" || len(q.Question) > 4096 || len(q.Evidence) > 4096 {
		return nil, errors.New("invalid business question from harness")
	}
	input := store.CheckpointQuestion{Trees: trees, Lease: job.LeaseToken, Sequence: &seq, Session: session, Question: q.Question, Evidence: q.Evidence}
	for ctx.Err() == nil {
		var reply struct {
			Answer *string `json:"answer"`
		}
		status, err := w.post(ctx, fmt.Sprintf("/internal/executions/%d/checkpoint-question", job.ExecutionID), input, &reply)
		if status == http.StatusOK {
			if err != nil || reply.Answer == nil {
				return nil, errors.New("invalid checkpoint question response")
			}
			if *reply.Answer != "" {
				return json.Marshal(map[string]string{"question": q.Question, "answer": *reply.Answer})
			}
		} else if !temporary(status) {
			return nil, fmt.Errorf("checkpoint question rejected (HTTP %d)", status)
		}
		wait(ctx)
	}
	return nil, ctx.Err()
}
