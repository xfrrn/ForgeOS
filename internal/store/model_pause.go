package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type ModelPause struct {
	Instance  int64      `json:"instance"`
	Session   string     `json:"session_id"`
	Requested time.Time  `json:"requested_at"`
	Parked    *time.Time `json:"parked_at,omitempty"`
	Resumed   *time.Time `json:"resumed_at,omitempty"`
	Index     string     `json:"index_tree,omitempty"`
	Worktree  string     `json:"worktree_tree,omitempty"`
}

func (s *Store) ControlModel(ctx context.Context, execution, instance int64, resume bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var task int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM executions e JOIN work_items t ON t.id=e.task_id WHERE e.id=$1 FOR UPDATE OF e,t`, execution).Scan(&task)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var parked, resumed bool
	err = tx.QueryRowContext(ctx, `SELECT parked_at IS NOT NULL,resumed_at IS NOT NULL FROM model_pauses WHERE execution_id=$1 AND instance=$2`, execution, instance).Scan(&parked, &resumed)
	found := err == nil
	if !found && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if found && (!resume || resumed) {
		return nil
	}
	var res sql.Result
	event := "model_pause_requested"
	if resume {
		if !parked {
			return ErrConflict
		}
		res, err = tx.ExecContext(ctx, `UPDATE work_items t SET status='RUNNING',waiting_execution_id=NULL FROM executions e
		WHERE e.id=$1 AND t.waiting_execution_id=e.id AND t.status='PAUSED' AND e.lease_until>clock_timestamp()
		AND e.result IS NULL AND e.abandoned_at IS NULL AND e.owner_cancel_requested_at IS NULL AND t.source_closure_id IS NULL`, execution)
		event = "model_resumed"
	} else {
		res, err = tx.ExecContext(ctx, `INSERT INTO model_pauses(execution_id,resource_step,instance,session_id)
		SELECT e.id,e.resource_step,$2,e.observed_process->>'session_id' FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.supports_model_pause AND e.resource_kind='model' AND e.result IS NULL AND e.abandoned_at IS NULL AND e.ci_candidate IS NULL
		AND t.status='RUNNING' AND t.source_closure_id IS NULL AND e.owner_cancel_requested_at IS NULL AND e.lease_until>clock_timestamp()
		AND e.observed_process->>'exited_at' IS NULL AND length(e.observed_process->>'session_id')>0
		AND (e.observed_process->>'instance')::bigint=$2 AND (e.observed_process->>'resource_step')::bigint=e.resource_step
		ON CONFLICT DO NOTHING`, execution, instance)
	}
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
	if resume {
		if _, err = tx.ExecContext(ctx, `UPDATE model_pauses SET resumed_at=clock_timestamp() WHERE execution_id=$1 AND instance=$2`, execution, instance); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES($1,$2,$3)`, task, execution, event); err != nil {
		return err
	}
	return tx.Commit()
}

type PausePoll struct {
	Lease          string         `json:"lease_token"`
	Step           int64          `json:"step"`
	Session        string         `json:"session_id,omitempty"`
	Trees          *WorktreeTrees `json:"trees,omitempty"`
	ResumeInstance string         `json:"resume_instance,omitempty"`
}

var ErrModelResumeClaimed = errors.New("another worker process claimed this model resume")

func (s *Store) PollModelPause(ctx context.Context, execution int64, input PausePoll) (*ModelPause, error) {
	if input.Step < 1 || len(input.Lease) != 64 || (input.Trees == nil) != (input.Session == "") {
		return nil, ErrConflict
	}
	if input.ResumeInstance != "" && (input.Trees == nil || len(input.ResumeInstance) != 64 || !validCommit(input.ResumeInstance)) {
		return nil, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(input.Lease))
	var task int64
	var kind, state string
	var cancelled, closed bool
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT t.id,e.resource_kind,t.status,e.owner_cancel_requested_at IS NOT NULL,t.source_closure_id IS NOT NULL,(SELECT to_jsonb(p) FROM model_pauses p WHERE p.execution_id=e.id AND p.resource_step=$3)
	FROM executions e JOIN work_items t ON t.id=e.task_id WHERE e.id=$1 AND e.lease_hash=$2 AND e.supports_model_pause
	AND e.resource_step=$3 AND e.result IS NULL AND e.abandoned_at IS NULL AND e.lease_until>clock_timestamp()
	AND (t.status='RUNNING' OR (t.status='PAUSED' AND t.waiting_execution_id=e.id))
	FOR UPDATE OF e,t`, execution, hash[:], input.Step).Scan(&task, &kind, &state, &cancelled, &closed, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	if cancelled {
		return nil, ErrOwnerStop
	}
	if closed {
		return nil, ErrSourceClosed
	}
	if raw == nil {
		if input.Trees != nil {
			return nil, ErrConflict
		}
		return nil, nil
	}
	var row struct {
		ModelPause
		ClaimRequired bool `json:"claim_required"`
	}
	if err = json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	p := &row.ModelPause
	if trees := input.Trees; trees != nil {
		if p.Session != input.Session || !validCommit(trees.Index) || !validCommit(trees.Worktree) || len(trees.Index) != len(trees.Worktree) {
			return nil, ErrConflict
		}
		if p.Parked != nil {
			if p.Index != trees.Index || p.Worktree != trees.Worktree || row.ClaimRequired != (input.ResumeInstance != "") {
				return nil, ErrConflict
			}
		} else {
			if state != "RUNNING" || kind != "" {
				return nil, ErrConflict
			}
			err = tx.QueryRowContext(ctx, `UPDATE model_pauses SET parked_at=clock_timestamp(),index_tree=$3,worktree_tree=$4,claim_required=$5
			WHERE execution_id=$1 AND resource_step=$2 RETURNING parked_at`, execution, input.Step, trees.Index, trees.Worktree, input.ResumeInstance != "").Scan(&p.Parked)
			if err != nil {
				return nil, err
			}
			p.Index, p.Worktree = trees.Index, trees.Worktree
			if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='PAUSED',waiting_execution_id=$2 WHERE id=$1`, task, execution); err != nil {
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES($1,$2,'model_paused')`, task, execution); err != nil {
				return nil, err
			}
		}
		if p.Resumed != nil && input.ResumeInstance != "" {
			claim := sha256.Sum256([]byte(input.ResumeInstance))
			var instance int64
			err = tx.QueryRowContext(ctx, `UPDATE model_pauses SET resume_claim=$3 WHERE execution_id=$1 AND resource_step=$2
				AND (resume_claim IS NULL OR resume_claim=$3) RETURNING instance`, execution, input.Step, claim[:]).Scan(&instance)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrModelResumeClaimed
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return p, tx.Commit()
}
