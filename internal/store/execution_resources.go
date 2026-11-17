package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
)

// A live pre-upgrade Issue has no phase information. Reserve both budgets until it settles.
const workerUsage = `SELECT count(*) AS reserved,
	count(*) FILTER (WHERE e.resource_kind='model' OR (NOT e.resource_managed AND t.kind='codex_issue')) AS model_reserved,
	count(*) FILTER (WHERE e.resource_kind='ci' OR (NOT e.resource_managed AND t.kind='codex_issue')) AS ci_reserved
	FROM executions e JOIN work_items t ON e.task_id=t.id
	WHERE e.worker=n.worker AND e.result IS NULL AND e.abandoned_at IS NULL`

type ResourceRequest struct {
	Lease   string `json:"lease_token"`
	Step    int64  `json:"step"`
	Kind    string `json:"kind"`
	Release bool   `json:"release,omitempty"`
}

// ReserveExecutionResource returns false without an error when the node's budget is full.
func (s *Store) ReserveExecutionResource(ctx context.Context, execution int64, r ResourceRequest) (bool, error) {
	if r.Step <= 0 || (r.Kind != "model" && r.Kind != "ci") || r.Lease == "" {
		return false, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if r.Kind == "model" {
		if _, err = tx.ExecContext(ctx, `LOCK TABLE model_budgets IN ROW SHARE MODE`); err != nil {
			return false, err
		}
	}
	hash := sha256.Sum256([]byte(r.Lease))
	var worker, kind, harness string
	var pool sql.NullString
	var step int64
	var limit int
	var sourceClosed, ownerStop bool
	err = tx.QueryRowContext(ctx, `SELECT n.worker, e.resource_step, e.resource_kind, e.model_budget,COALESCE(NULLIF(t.spec->>'harness',''),'codex'),
		COALESCE(CASE WHEN $3='model' THEN n.model_slots ELSE n.ci_slots END,n.slots),t.source_closure_id IS NOT NULL,(e.owner_cancel_requested_at IS NOT NULL OR e.ci_pause_requested_at IS NOT NULL)
		FROM executions e JOIN worker_nodes n ON n.worker=e.worker JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.lease_hash=$2 AND e.resource_managed AND e.result IS NULL AND e.abandoned_at IS NULL
		AND t.waiting_execution_id IS NULL AND ($3='ci' OR t.kind='codex_issue')
		FOR UPDATE OF n,e,t`, execution, hash[:], r.Kind).Scan(&worker, &step, &kind, &pool, &harness, &limit, &sourceClosed, &ownerStop)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrConflict
	}
	if err != nil {
		return false, err
	}
	if ownerStop && !r.Release {
		return false, ErrOwnerStop
	}
	if sourceClosed && !r.Release {
		return false, ErrSourceClosed
	}
	if r.Release {
		if r.Step != step || (kind != "" && kind != r.Kind) {
			return false, ErrConflict
		}
		kind = ""
		pool = sql.NullString{}
	} else if step != r.Step || kind != r.Kind {
		if r.Step != step+1 || kind != "" {
			return false, ErrConflict
		}
		var used int
		err := tx.QueryRowContext(ctx, `SELECT CASE WHEN $2='model' THEN usage.model_reserved ELSE usage.ci_reserved END
			FROM worker_nodes n CROSS JOIN LATERAL (`+workerUsage+`) usage WHERE n.worker=$1
			AND EXISTS(SELECT 1 FROM executions WHERE id=$3 AND lease_until>clock_timestamp())`, worker, r.Kind, execution).Scan(&used)
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrConflict
		}
		if err != nil {
			return false, err
		}
		if used >= limit {
			return false, nil
		}
		if r.Kind == "model" {
			var available bool
			pool, available, err = modelSlot(ctx, tx, harness, worker, "", 0)
			if err != nil || !available {
				return false, err
			}
		}
		step, kind = r.Step, r.Kind
	}
	res, err := tx.ExecContext(ctx, `UPDATE executions SET resource_step=$2,resource_kind=$3,model_budget=$4
		WHERE id=$1 AND lease_until>clock_timestamp()`, execution, step, kind, pool)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n != 1 {
		return false, ErrConflict
	}
	return true, tx.Commit()
}
