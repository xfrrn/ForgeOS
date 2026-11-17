package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type InterruptedWriterTurn struct {
	Source  HarnessProcess `json:"source"`
	Missing HarnessProcess `json:"missing"`
	Trees   WorktreeTrees  `json:"trees"`
}

func recordInterruptedWriter(ctx context.Context, tx *sql.Tx, id int64, seq int, proof InterruptedWriterTurn) error {
	source, missing, trees := proof.Source, proof.Missing, proof.Trees
	if !source.Valid() || !missing.Valid() || source.Session == "" || source.ExitCode == nil || *source.ExitCode == 0 ||
		missing.Session != "" || missing.ExitCode == nil || *missing.ExitCode != 1 || missing.Instance <= source.Instance ||
		missing.Started.Before(*source.Exited) || missing.Harness != source.Harness || missing.Version != source.Version ||
		!validCommit(trees.Index) || !validCommit(trees.Worktree) || len(trees.Index) != len(trees.Worktree) {
		return ErrConflict
	}
	var observed []byte
	err := tx.QueryRowContext(ctx, `SELECT e.observed_process FROM executions e JOIN work_items t ON t.id=e.task_id
		JOIN execution_checkpoints c ON c.execution_id=e.id AND c.sequence=$2
		WHERE e.id=$1 AND e.supports_model_pause AND e.resource_kind='model' AND e.lease_until>clock_timestamp()
		AND COALESCE(t.spec->>'mode','')<>'plan' AND NOT (t.spec ? 'verify_commit')
		AND COALESCE(NULLIF(t.spec->>'harness',''),'codex')=$3 AND length(t.spec->>'base_commit')=$4
		AND c.session_id IS NULL AND NOT EXISTS(SELECT 1 FROM model_pauses p WHERE p.execution_id=e.id AND p.resumed_at IS NULL)`,
		id, seq, source.Harness, len(trees.Index)).Scan(&observed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if len(observed) > 0 {
		var prior HarnessProcess
		if err := json.Unmarshal(observed, &prior); err != nil {
			return err
		}
		switch {
		case prior.Instance <= source.Instance:
			if !source.follows(prior) {
				return ErrConflict
			}
		case prior.Instance != missing.Instance || !missing.follows(prior):
			return ErrConflict
		}
	}
	src, err := json.Marshal(source)
	if err != nil {
		return err
	}
	miss, err := json.Marshal(missing)
	if err != nil {
		return err
	}
	var recorded int64
	err = tx.QueryRowContext(ctx, `INSERT INTO interrupted_writer_turns AS p
		(execution_id,sequence,source_instance,session_id,source_process,missing_process,index_tree,worktree_tree)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(execution_id,source_instance) DO UPDATE SET sequence=p.sequence
		WHERE (p.sequence,p.source_process,p.missing_process,p.index_tree,p.worktree_tree)=
		      (excluded.sequence,excluded.source_process,excluded.missing_process,excluded.index_tree,excluded.worktree_tree)
		RETURNING source_instance`, id, seq, source.Instance, source.Session, string(src), string(miss), trees.Index, trees.Worktree).Scan(&recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}
