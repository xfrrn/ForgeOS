package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
)

type NativeTurn struct {
	Instance int64  `json:"instance"`
	Session  string `json:"session_id"`
	Turn     string `json:"turn_id"`
}

type NativeTurnInput struct {
	NativeTurn
	OwnerInstruction
}

type TurnInputPoll struct {
	NativeTurn
	Lease   string `json:"lease_token"`
	Key     string `json:"key,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Claim   string `json:"claim_key,omitempty"`
}

func (s *Store) QueueTurnInput(ctx context.Context, execution int64, input NativeTurnInput) error {
	if !input.Valid() || !feedbackRequest.MatchString(input.Key) || input.Instance < 1 || !ValidAppTurnID(input.Turn) || !ValidAppTurnID(input.Session) {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var task int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 FOR UPDATE OF e,t`, execution).Scan(&task)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var prior NativeTurnInput
	err = tx.QueryRowContext(ctx, `SELECT instance,session_id,turn_id,key,body FROM native_turn_inputs
		WHERE execution_id=$1 AND key=$2`, execution, input.Key).Scan(&prior.Instance, &prior.Session, &prior.Turn, &prior.Key, &prior.Body)
	if err == nil {
		if prior != input {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO native_turn_inputs(execution_id,instance,session_id,turn_id,key,body)
		SELECT execution_id,instance,session_id,turn_id,$5,$6 FROM open_native_input_turns
		WHERE execution_id=$1 AND instance=$2 AND session_id=$3 AND turn_id=$4
		AND (SELECT count(*) FROM native_turn_inputs WHERE execution_id=$1 AND instance=$2)<8`,
		execution, input.Instance, input.Session, input.Turn, input.Key, input.Body)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return tx.Commit()
}

// Claim once before writing to native stdin. App Server does not deduplicate client message IDs.
func (s *Store) PollTurnInput(ctx context.Context, execution int64, poll TurnInputPoll) (*OwnerInstruction, error) {
	if len(poll.Lease) != 64 || poll.Instance < 1 || !ValidAppTurnID(poll.Session) || !ValidAppTurnID(poll.Turn) {
		return nil, ErrConflict
	}
	hash := sha256.Sum256([]byte(poll.Lease))
	args := []any{execution, hash[:], poll.Instance, poll.Session, poll.Turn}
	if poll.Key != "" || poll.Outcome != "" {
		if !feedbackRequest.MatchString(poll.Key) || (poll.Outcome != "accepted" && poll.Outcome != "rejected") {
			return nil, ErrConflict
		}
		// A native response can reach the controller after the lease or turn has ended.
		res, err := s.db.ExecContext(ctx, `UPDATE native_turn_inputs i SET outcome=$7,confirmed_at=coalesce(i.confirmed_at,clock_timestamp())
			FROM executions e WHERE e.id=$1 AND e.lease_hash=$2 AND i.execution_id=e.id AND i.instance=$3
			AND i.session_id=$4 AND i.turn_id=$5 AND i.key=$6 AND i.claimed_at IS NOT NULL AND (i.outcome IS NULL OR i.outcome=$7)`,
			append(args, poll.Key, poll.Outcome)...)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err == nil && n != 1 {
			err = ErrConflict
		}
		return nil, err
	}
	if len(poll.Claim) != 32 || strings.Trim(poll.Claim, "0123456789abcdef") != "" {
		return nil, ErrConflict
	}
	var item OwnerInstruction
	err := s.db.QueryRowContext(ctx, `WITH next AS (
		SELECT i.id FROM native_turn_inputs i JOIN open_native_input_turns a USING(execution_id,instance,session_id,turn_id)
		JOIN executions e ON e.id=a.execution_id WHERE e.id=$1 AND e.lease_hash=$2 AND i.instance=$3
		AND i.session_id=$4 AND i.turn_id=$5 AND (i.claim_key IS NULL OR i.claim_key=$6)
		ORDER BY i.claim_key NULLS LAST,i.id LIMIT 1 FOR UPDATE OF i)
		UPDATE native_turn_inputs i SET claimed_at=coalesce(i.claimed_at,clock_timestamp()),claim_key=$6 FROM next
		WHERE i.id=next.id AND (i.claim_key IS NULL OR i.claim_key=$6)
		RETURNING i.key,i.body`, append(args, poll.Claim)...).Scan(&item.Key, &item.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}
