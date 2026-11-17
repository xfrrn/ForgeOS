package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

// Walk back only the linked sessions that led to this checkpoint confirmation.
func (w *worker) checkpointHandoffs(ctx context.Context, job store.Job, session string) ([]store.SessionHandoff, error) {
	key := sha256.Sum256([]byte(job.LeaseToken))
	rows, err := w.journal.QueryContext(ctx, `SELECT p.instance,p.session_id,h.previous_session,
		(SELECT prior.instance FROM harness_processes prior WHERE prior.lease_hash=p.lease_hash AND prior.instance<p.instance
		 AND prior.session_id=h.previous_session ORDER BY prior.instance DESC LIMIT 1)
		FROM harness_processes p JOIN harness_session_handoffs h ON h.instance=p.instance
		WHERE p.lease_hash=? AND p.session_id IS NOT NULL ORDER BY p.instance DESC`, key[:])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []store.SessionHandoff
	for rows.Next() {
		var link store.SessionHandoff
		if err := rows.Scan(&link.Instance, &link.Session, &link.From, &link.SourceInstance); err != nil {
			return nil, err
		}
		if link.Session == session {
			links = append(links, link)
			session = link.From
		}
	}
	slices.Reverse(links)
	return links, rows.Err()
}

type completedTurn struct {
	Sequence     int                  `json:"sequence"`
	Session      string               `json:"session_id"`
	Comments     []store.CommentInput `json:"discussion,omitempty"`
	Instructions json.RawMessage      `json:"owner_instructions,omitempty"`
	Answer       json.RawMessage      `json:"decision_response,omitempty"`
}

func (w *worker) priorTurnInputs(ctx context.Context, job store.Job, sequence int) (string, error) {
	var turns []completedTurn
	for seq := 0; seq < sequence; seq++ {
		page, err := w.readCheckpoint(ctx, job, seq, "", true)
		if err != nil {
			return "", err
		}
		if page.SessionID == "" {
			return "", fmt.Errorf("checkpoint %d has no confirmed native session", seq)
		}
		turn := completedTurn{Sequence: seq, Session: page.SessionID, Comments: page.Comments, Instructions: page.OwnerInstructions}
		if len(page.AnswerMessages) > 0 {
			turn.Answer, err = w.historicalAnswer(page.AnswerMessages, job, seq-1)
			if err != nil {
				return "", err
			}
		}
		turns = append(turns, turn)
	}
	raw, err := json.Marshal(turns)
	return string(raw), err
}

func (w *worker) historicalAnswer(raw json.RawMessage, job store.Job, sequence int) (json.RawMessage, error) {
	var pair []json.RawMessage
	if err := json.Unmarshal(raw, &pair); err != nil || len(pair) != 2 {
		return nil, errors.New("incomplete historical decision messages")
	}
	m, err := readScopedMessage(pair[0], job)
	if err != nil {
		return nil, err
	}
	var q nativeQuestion
	q.Question, _ = m.Payload["question"].(string)
	q.Evidence, _ = m.Payload["evidence"].(string)
	if m.Payload["request_kind"] == "decision_lookup" {
		ref, _ := m.Payload["reference"].(string)
		q.ReferenceDecisionID, err = strconv.ParseInt(strings.TrimPrefix(ref, "decision:"), 10, 64)
		if err != nil || q.ReferenceDecisionID <= 0 {
			return nil, errors.New("invalid historical decision reference")
		}
	}
	return checkpointAnswer(pair, w.id, job, sequence, q)
}
