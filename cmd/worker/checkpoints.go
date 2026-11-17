package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"forgeos/internal/store"
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

func (w *worker) checkpoint(ctx context.Context, job store.Job, seq int, session string) (store.CheckpointPage, error) {
	return w.readCheckpoint(ctx, job, seq, session, false)
}

func (w *worker) readCheckpoint(ctx context.Context, job store.Job, seq int, session string, history bool) (store.CheckpointPage, error) {
	var batch store.CheckpointPage
	if !job.SupportsCheckpoints {
		return batch, nil
	}

	request := store.CheckpointRequest{Lease: job.LeaseToken, Sequence: &seq, Session: session, History: history}
	if job.CheckpointHandoff && session != "" && w.journal != nil {
		var err error
		request.Handoffs, err = w.checkpointHandoffs(ctx, job, session)
		if err != nil {
			return batch, err
		}
	}
	path := fmt.Sprintf("/internal/executions/%d/checkpoints?queue_next=1", job.ExecutionID)
	switch job.InstructionProtocol {
	case "1.0":
		path += "&commands=1.0"
	case "":
	default:
		return batch, errors.New("unsupported checkpoint command protocol")
	}
	for ctx.Err() == nil {
		var page store.CheckpointPage
		status, err := w.post(ctx, path, request, &page)
		if temporary(status) {
			wait(ctx)
			continue
		}
		if err != nil {
			return batch, err
		}
		if session != "" && status == http.StatusNoContent {
			return batch, nil
		}
		if status != http.StatusOK || page.ExecutionID != job.ExecutionID || page.Sequence != seq {
			return batch, fmt.Errorf("checkpoint %d not acknowledged (HTTP %d)", seq, status)
		}
		if request.After == 0 {
			batch.Checkpoint = page.Checkpoint
			batch.AnswerMessages = page.AnswerMessages
			batch.OwnerInstructions = page.OwnerInstructions
			if job.InstructionProtocol == "1.0" {
				batch.OwnerInstructions, err = checkpointCommands(page, job, w.id)
				if err != nil {
					return batch, err
				}
			}
		}
		batch.Comments = append(batch.Comments, page.Comments...)
		if !page.More {
			return batch, nil
		}
		if len(page.Comments) == 0 || page.Comments[len(page.Comments)-1].EventID <= request.After {
			return batch, fmt.Errorf("checkpoint %d cursor did not advance", seq)
		}
		request.After = page.Comments[len(page.Comments)-1].EventID
	}
	return batch, ctx.Err()
}

// Only the adapter's final assistant message may ask for a business decision.
type nativeQuestion struct {
	Question            string `json:"forgeos_question"`
	Evidence            string `json:"evidence"`
	ReferenceDecisionID int64
	requested           bool
}

func (q *nativeQuestion) read(message string) {
	*q = nativeQuestion{}
	message = strings.TrimSpace(message)
	if strings.HasPrefix(message, "```json") && strings.HasSuffix(message, "```") {
		message = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(message, "```json"), "```"))
	}
	var fields map[string]json.RawMessage
	if !strings.HasPrefix(message, "{") || json.Unmarshal([]byte(message), &fields) != nil {
		return
	}
	if ref := fields["forgeos_lookup_decision"]; ref != nil {
		q.requested = true
		if len(fields) == 1 && json.Unmarshal(ref, &q.ReferenceDecisionID) == nil && q.ReferenceDecisionID > 0 {
			q.Question, q.Evidence = store.DecisionLookupText(q.ReferenceDecisionID)
		}
		return
	}
	if fields["forgeos_question"] == nil {
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
	if q.ReferenceDecisionID != 0 && (!job.DecisionLookups || job.DecisionProtocol != "1.0" || !job.ResumeQuestions) {
		return nil, errors.New("decision lookup unavailable")
	}
	input := store.CheckpointQuestion{ReferenceDecisionID: q.ReferenceDecisionID, Trees: trees, Lease: job.LeaseToken, Sequence: &seq, Session: session, Question: q.Question, Evidence: q.Evidence}
	if job.QuestionHandoff && w.journal != nil {
		key := sha256.Sum256([]byte(job.LeaseToken))
		err := w.journal.QueryRowContext(ctx, `SELECT instance FROM
			(SELECT instance,session_id,exit_code FROM harness_processes WHERE lease_hash=? ORDER BY instance DESC LIMIT 1)
			WHERE session_id=? AND exit_code=0`, key[:], session).Scan(&input.SourceInstance)
		if err != nil {
			return nil, fmt.Errorf("identify the process that asked at checkpoint %d: %w", seq, err)
		}
	}
	path := fmt.Sprintf("/internal/executions/%d/checkpoint-question", job.ExecutionID)
	switch job.DecisionProtocol {
	case "1.0":
		path += "?acp=1.0"
	case "":
	default:
		return nil, errors.New("unsupported decision message protocol")
	}
	if job.ResumeQuestions {
		var nonce [32]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		input.ResumeInstance = hex.EncodeToString(nonce[:])
	}
	for ctx.Err() == nil {
		var reply struct {
			Answer   *string           `json:"answer"`
			Messages []json.RawMessage `json:"messages"`
		}
		status, err := w.post(ctx, path, input, &reply)
		if status == http.StatusOK {
			if err != nil {
				return nil, err
			}
			if job.DecisionProtocol == "1.0" {
				if reply.Messages == nil {
					return nil, errors.New("missing checkpoint decision messages")
				}
				if len(reply.Messages) > 0 {
					return checkpointAnswer(reply.Messages, w.id, job, seq, q)
				}
			} else if reply.Answer == nil {
				return nil, errors.New("invalid checkpoint question response")
			} else if *reply.Answer != "" {
				return json.Marshal(map[string]string{"question": q.Question, "answer": *reply.Answer})
			}
		} else if status == http.StatusPreconditionFailed {
			return nil, store.ErrQuestionClaimed
		} else if status == http.StatusConflict && job.ResumeQuestions {
			return nil, store.ErrConflict
		} else if !temporary(status) {
			return nil, fmt.Errorf("checkpoint question rejected (HTTP %d)", status)
		}
		wait(ctx)
	}
	return nil, ctx.Err()
}
