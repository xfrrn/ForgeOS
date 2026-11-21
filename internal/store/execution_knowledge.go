package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
)

// KnowledgeScope resolves the project and repository version from the running
// implementation. Worker-supplied project IDs and commits cannot change either.
func (s *Store) KnowledgeScope(ctx context.Context, id int64, lease string) (int64, string, error) {
	return knowledgeScope(ctx, s.db, id, lease)
}

func knowledgeScope(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64, lease string) (int64, string, error) {
	var project int64
	var commit string
	hash := sha256.Sum256([]byte(lease))
	err := db.QueryRowContext(ctx, `SELECT t.project_id,t.spec->>'base_commit'
		FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.lease_hash=$2 AND e.knowledge_tools AND e.lease_until>clock_timestamp()
		  AND t.kind='codex_issue' AND t.status='RUNNING' AND t.source_closure_id IS NULL
		  AND COALESCE(t.spec->>'mode','')<>'plan' AND COALESCE(t.spec->>'verify_commit','')=''
		  AND e.result IS NULL AND e.abandoned_at IS NULL AND e.ci_candidate IS NULL
		  AND e.owner_cancel_requested_at IS NULL AND e.ci_pause_requested_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM model_pauses p WHERE p.execution_id=e.id AND p.resumed_at IS NULL)`,
		id, hash[:]).Scan(&project, &commit)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrConflict
	}
	return project, commit, err
}

// The same locks serialize checkpoint creation, cancellation, citation and
// candidate finalization. Recheck the lease against the clock after locking.
func (s *Store) beginKnowledgeWrite(ctx context.Context, id int64, lease, kind, ref string) (*sql.Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(lease))
	var task int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.lease_hash=$2 FOR UPDATE OF e,t`, id, hash[:]).Scan(&task)
	if err == nil {
		var project int64
		project, _, err = knowledgeScope(ctx, tx, id, lease)
		if err == nil && kind == SharedKnowledgeKind {
			err = lockReceivedKnowledge(ctx, tx, project, ref)
			if err == nil {
				// Waiting for publication can outlast an otherwise valid lease.
				_, _, err = knowledgeScope(ctx, tx, id, lease)
			}
		}
	}
	if err != nil {
		tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConflict
		}
		return nil, err
	}
	return tx, nil
}

// RememberKnowledgeRead only accepts evidence resolved by the control plane.
// Returning the first snapshot on retry keeps the exact text read by the CLI.
func (s *Store) RememberKnowledgeRead(ctx context.Context, id int64, lease string, evidence KnowledgeEvidence) (KnowledgeEvidence, error) {
	var stored KnowledgeEvidence
	raw, err := json.Marshal(evidence)
	if err != nil {
		return stored, err
	}
	tx, err := s.beginKnowledgeWrite(ctx, id, lease, evidence.Kind, evidence.Ref)
	if err != nil {
		return stored, err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(evidence.Ref))
	_, err = tx.ExecContext(ctx, `INSERT INTO execution_knowledge_reads(execution_id,kind,ref_hash,evidence)
		VALUES ($1,$2,$3,$4) ON CONFLICT (execution_id,kind,ref_hash) DO NOTHING`, id, evidence.Kind, hash[:], string(raw))
	if err != nil {
		return stored, err
	}
	err = tx.QueryRowContext(ctx, `SELECT evidence FROM execution_knowledge_reads
		WHERE execution_id=$1 AND kind=$2 AND ref_hash=$3 AND evidence->>'ref'=$4`, id, evidence.Kind, hash[:], evidence.Ref).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return stored, ErrConflict
	}
	if err != nil {
		return stored, err
	}
	if err = json.Unmarshal(raw, &stored); err != nil {
		return stored, err
	}
	return stored, tx.Commit()
}

func (s *Store) CiteKnowledge(ctx context.Context, id int64, lease, kind, ref string) (KnowledgeEvidence, error) {
	var stored KnowledgeEvidence
	tx, err := s.beginKnowledgeWrite(ctx, id, lease, kind, ref)
	if err != nil {
		return stored, err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(ref))
	var raw []byte
	err = tx.QueryRowContext(ctx, `UPDATE execution_knowledge_reads SET
			checkpoint_sequence=CASE WHEN cited_at IS NULL THEN
				(SELECT coalesce(max(sequence),0) FROM execution_checkpoints WHERE execution_id=$1) ELSE checkpoint_sequence END,
			cited_at=coalesce(cited_at,clock_timestamp())
		WHERE execution_id=$1 AND kind=$2 AND ref_hash=$3 AND evidence->>'ref'=$4
		RETURNING evidence`, id, kind, hash[:], ref).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return stored, ErrNotFound
	}
	if err != nil {
		return stored, err
	}
	if err = json.Unmarshal(raw, &stored); err != nil {
		return stored, err
	}
	return stored, tx.Commit()
}
