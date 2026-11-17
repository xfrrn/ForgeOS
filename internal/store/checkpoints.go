package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Checkpoint struct {
	ExecutionID  int64     `json:"execution_id"`
	Sequence     int       `json:"sequence"`
	SourceTaskID int64     `json:"source_task_id"`
	After        int64     `json:"after_event"`
	Through      int64     `json:"through_event"`
	At           time.Time `json:"at"`
	SessionID    string    `json:"session_id,omitempty"`
}
type CheckpointSummary struct {
	Checkpoint
	CommentCount     int             `json:"comment_count"`
	NextInstructions json.RawMessage `json:"next_instructions,omitempty"`
}
type CommentInput struct {
	EventID   int64  `json:"event_id"`
	CommentID int64  `json:"comment_id"`
	URL       string `json:"source"`
	Body      string `json:"body"`
}

type OwnerInstruction struct {
	Key  string `json:"key"`
	Body string `json:"body"`
}

func (input OwnerInstruction) Valid() bool {
	return len(input.Key) > 0 && len(input.Key) <= 64 && strings.Trim(input.Key, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") == "" &&
		strings.TrimSpace(input.Body) != "" && !strings.ContainsRune(input.Body, 0) && len(input.Body) <= 4096
}

func (s *Store) QueueInstruction(ctx context.Context, id int64, seq int, input OwnerInstruction) error {
	if seq < 0 || seq > 2147483646 || !input.Valid() {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT c.next_instructions FROM executions e JOIN execution_checkpoints c ON c.execution_id=e.id
		WHERE e.id=$1 AND c.sequence=$2 FOR UPDATE OF e,c`, id, seq).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	var queued []OwnerInstruction
	if err = json.Unmarshal(raw, &queued); err != nil {
		return err
	}
	for _, entry := range queued {
		if entry.Key == input.Key {
			if entry != input {
				return ErrConflict
			}
			return nil
		}
	}
	if len(queued) >= 8 {
		return ErrConflict
	}
	raw, err = json.Marshal(append(queued, input))
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE execution_checkpoints c SET next_instructions=$3 FROM open_instruction_queues q
		WHERE c.execution_id=$1 AND c.sequence=$2 AND q.execution_id=c.execution_id AND q.sequence=c.sequence`, id, seq, string(raw))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return tx.Commit()
}

type CheckpointPage struct {
	Checkpoint
	TaskID            int64             `json:"-"`
	Commands          []json.RawMessage `json:"commands,omitempty"`
	Comments          []CommentInput    `json:"comments"`
	OwnerInstructions json.RawMessage   `json:"owner_instructions,omitempty"`
	More              bool              `json:"more"`
	AnswerMessages    json.RawMessage   `json:"answer_messages,omitempty"`
}

type SessionHandoff struct {
	From           string `json:"from"`
	Session        string `json:"session"`
	SourceInstance int64  `json:"source_instance"`
	Instance       int64  `json:"instance"`
}

type CheckpointRequest struct {
	Lease    string           `json:"lease_token"`
	Sequence *int             `json:"sequence"`
	After    int64            `json:"after"`
	Session  string           `json:"session_id"`
	History  bool             `json:"history,omitempty"`
	Handoffs []SessionHandoff `json:"handoffs,omitempty"`
}

func (s *Store) ReadCheckpoint(ctx context.Context, id int64, lease string, seq int, after int64, session string, supportsQueue bool) (CheckpointPage, error) {
	return s.WorkerCheckpoint(ctx, id, CheckpointRequest{Lease: lease, Sequence: &seq, After: after, Session: session}, supportsQueue)
}

func (s *Store) WorkerCheckpoint(ctx context.Context, id int64, request CheckpointRequest, supportsQueue bool) (CheckpointPage, error) {
	var page CheckpointPage
	if request.Sequence == nil || (request.History && request.Session != "") || (len(request.Handoffs) != 0 && request.Session == "") {
		return page, ErrConflict
	}
	seq, after, session, lease := *request.Sequence, request.After, request.Session, request.Lease
	if seq < 0 || after < 0 || len(session) > 128 || strings.Trim(session, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return page, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return page, err
	}
	defer tx.Rollback()
	var digest []byte
	var source int64
	err = tx.QueryRowContext(ctx, `SELECT e.lease_hash,COALESCE(t.goal_id,t.id),t.id FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.uses_checkpoints AND t.kind='codex_issue' AND t.status='RUNNING' AND t.source_closure_id IS NULL AND e.owner_cancel_requested_at IS NULL AND e.ci_pause_requested_at IS NULL
		AND e.result IS NULL AND e.abandoned_at IS NULL AND e.lease_until>now() FOR UPDATE OF e,t`, id).Scan(&digest, &source, &page.TaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return page, ErrConflict
	}
	if err != nil {
		return page, err
	}
	hash := sha256.Sum256([]byte(lease))
	if subtle.ConstantTimeCompare(hash[:], digest) != 1 {
		return page, ErrConflict
	}
	var instructions []byte
	err = tx.QueryRowContext(ctx, `SELECT NULLIF(next_instructions,'[]') FROM execution_checkpoints WHERE execution_id=$1 AND sequence=$2::integer-1`, id, seq).Scan(&instructions)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return page, err
	}
	if len(instructions) > 0 && !supportsQueue {
		return page, ErrConflict
	}
	if after == 0 {
		page.OwnerInstructions = instructions
	}
	if session != "" {
		// A native turn may be reported repeatedly after a dropped HTTP response.
		for i, link := range request.Handoffs {
			if (i > 0 && (request.Handoffs[i-1].Session != link.From || request.Handoffs[i-1].Instance > link.SourceInstance)) ||
				(i == len(request.Handoffs)-1 && link.Session != session) {
				return page, ErrConflict
			}
			if err := recordCheckpointHandoff(ctx, tx, id, seq, link); err != nil {
				return page, err
			}
		}
		var stored int
		err = tx.QueryRowContext(ctx, `UPDATE execution_checkpoints SET session_id=$3 WHERE execution_id=$1 AND sequence=$2 AND (session_id IS NULL OR session_id=$3)
   AND NOT EXISTS(SELECT 1 FROM execution_checkpoints prior WHERE prior.execution_id=$1 AND prior.sequence=$2::integer-1
       AND prior.next_instructions<>'[]' AND NOT COALESCE(checkpoint_session_continues($1,$2,prior.session_id,$3),false))
   AND NOT EXISTS(SELECT 1 FROM decision_questions q JOIN execution_checkpoints asked ON asked.execution_id=q.execution_id AND asked.sequence=q.checkpoint_sequence
       WHERE q.execution_id=$1 AND q.checkpoint_sequence<$2 AND NOT checkpoint_session_continues($1,$2,asked.session_id,$3)) RETURNING sequence`, id, seq, session).Scan(&stored)
		if errors.Is(err, sql.ErrNoRows) {
			return page, ErrConflict
		}
		if err != nil {
			return page, err
		}
		return page, tx.Commit()
	}
	if !request.History {
		_, err = tx.ExecContext(ctx, `INSERT INTO execution_checkpoints(execution_id,sequence,source_task_id,after_event,through_event,supports_queue)
		SELECT $1,$2,$3,COALESCE(p.through_event,0),COALESCE((SELECT max(id) FROM issue_comment_events WHERE task_id=$3),0),$4
		FROM (SELECT 1) seed LEFT JOIN execution_checkpoints p ON p.execution_id=$1 AND p.sequence=$2::integer-1
		WHERE $2=0 OR p.session_id IS NOT NULL ON CONFLICT DO NOTHING`, id, seq, source, supportsQueue)
		if err != nil {
			return page, err
		}
	}
	err = tx.QueryRowContext(ctx, `SELECT execution_id,sequence,source_task_id,after_event,through_event,at,COALESCE(session_id,'') FROM execution_checkpoints WHERE execution_id=$1 AND sequence=$2`, id, seq).Scan(
		&page.ExecutionID, &page.Sequence, &page.SourceTaskID, &page.After, &page.Through, &page.At, &page.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return page, ErrConflict
	}
	if err != nil {
		return page, err
	}
	if (request.History && page.SessionID == "") || after > page.Through {
		return page, ErrConflict
	}
	if page.After == page.Through {
		page.Comments = []CommentInput{}
		return page, tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,comment_id,payload->>'html_url',payload->>'body' FROM checkpoint_comments
		WHERE execution_id=$1 AND sequence=$2 AND id>$3 ORDER BY id LIMIT 9`, id, seq, after)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	page.Comments = []CommentInput{}
	size := 0
	for rows.Next() {
		var c CommentInput
		if err = rows.Scan(&c.EventID, &c.CommentID, &c.URL, &c.Body); err != nil {
			return page, err
		}
		raw, err := json.Marshal(c)
		if err != nil {
			return page, err
		}
		size += len(raw)
		if len(page.Comments) > 0 && (size > 1<<20 || len(page.Comments) == 8) {
			page.More = true
			break
		}
		page.Comments = append(page.Comments, c)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if err = rows.Close(); err != nil {
		return page, err
	}
	return page, tx.Commit()
}

func recordCheckpointHandoff(ctx context.Context, tx *sql.Tx, id int64, seq int, link SessionHandoff) error {
	if link.SourceInstance <= 0 || link.Instance <= link.SourceInstance || link.From == link.Session || link.From == "" || link.Session == "" ||
		len(link.Session) > 128 || strings.Trim(link.Session, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return ErrConflict
	}
	var stored int
	err := tx.QueryRowContext(ctx, `UPDATE model_pauses p SET handoff_session=$4,handoff_instance=$5,handoff_sequence=COALESCE(handoff_sequence,$6)
		FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE p.execution_id=$1 AND e.id=p.execution_id AND COALESCE(t.spec->>'mode','')<>'plan' AND p.instance=$2
		AND p.session_id=$3 AND p.resumed_at IS NOT NULL AND (
		    (p.handoff_session=$4 AND p.handoff_instance=$5 AND p.handoff_sequence<=$6)
		    OR (p.handoff_session IS NULL AND NOT checkpoint_session_continues($1,$6,$4,$3)
		       AND EXISTS(SELECT 1 FROM execution_checkpoints c WHERE c.execution_id=e.id AND c.sequence=$6 AND c.session_id IS NULL AND c.at<=p.requested_at)
		       AND NOT EXISTS(SELECT 1 FROM execution_checkpoints c WHERE c.execution_id=e.id AND c.sequence>$6 AND c.at<=p.requested_at)))
		RETURNING handoff_sequence`, id, link.SourceInstance, link.From, link.Session, link.Instance, seq).Scan(&stored)
	var constraint *pq.Error
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &constraint) && constraint.Code == "23505") {
		return ErrConflict
	}
	return err
}

func (s *Store) Checkpoints(ctx context.Context, taskID, executionID int64) ([]CheckpointSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.execution_id,c.sequence,c.source_task_id,c.after_event,c.through_event,c.at,COALESCE(c.session_id,''),NULLIF(c.next_instructions,'[]'),
		(SELECT count(*) FROM checkpoint_comments i WHERE i.execution_id=c.execution_id AND i.sequence=c.sequence)
		FROM execution_checkpoints c JOIN executions e ON e.id=c.execution_id WHERE e.task_id=$1 AND e.id=$2 ORDER BY c.sequence`, taskID, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CheckpointSummary{}
	for rows.Next() {
		var c CheckpointSummary
		var instructions []byte
		if err = rows.Scan(&c.ExecutionID, &c.Sequence, &c.SourceTaskID, &c.After, &c.Through, &c.At, &c.SessionID, &instructions, &c.CommentCount); err != nil {
			return nil, err
		}
		c.NextInstructions = instructions
		out = append(out, c)
	}
	return out, rows.Err()
}
