package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type knowledgeClaimTarget struct{ role, table, result, lease, until string }

var knowledgeClaimTargets = map[string]knowledgeClaimTarget{
	"extract":         {"curator", "material_curations", "extraction", "extraction_lease", "extraction_until"},
	"curation_review": {"management", "material_curations", "review", "review_lease", "review_until"},
	"share_review":    {"management", "knowledge_shares", "review", "review_lease", "review_until"},
}

// column and kind are constants supplied by the two knowledge claim queries.
func knowledgeAttemptEligible(ctx context.Context, column, kind string) string {
	binding, ok := knowledgeAttemptBinding(ctx)
	if !ok {
		return ""
	}
	return fmt.Sprintf(` AND NOT EXISTS (SELECT 1 FROM knowledge_model_attempts a
        WHERE a.project_id=%d AND a.binding_revision=%d AND a.model_revision='%s' AND a.kind='%s'
        AND a.job_id=%s AND (a.attempts>=3 OR a.retry_at>clock_timestamp())) `, binding.ProjectID, binding.Revision, binding.ModelRevision, kind, column)
}

// ReserveKnowledgeModelAttempt is idempotent for duplicate delivery of a lease:
// only the caller which inserted or advanced the row may call the provider.
func (s *Store) ReserveKnowledgeModelAttempt(ctx context.Context, binding KnowledgeRoleModel, kind string, job int64, lease string) error {
	target, known := knowledgeClaimTargets[kind]
	b, direct := knowledgeAttemptBinding(ctx)
	if !direct || b != binding || !known || binding.Role != target.role {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	allowed, err := knowledgeModelAllowed(ctx, tx, binding.ProjectID, binding.Role)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrConflict
	}
	var n int
	err = tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE project_id=$1 AND id=$2 AND %s=$3
		AND %s IS NULL AND %s>clock_timestamp() FOR SHARE`, target.table, target.lease, target.result, target.until), binding.ProjectID, job, lease).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO knowledge_model_attempts(project_id,binding_revision,model_revision,kind,job_id,attempts,lease,retry_at)
        VALUES($1,$2,$3,$4,$5,1,$6,clock_timestamp()+interval '6 minutes')
        ON CONFLICT (project_id,binding_revision,model_revision,kind,job_id) DO UPDATE
        SET attempts=knowledge_model_attempts.attempts+1,lease=excluded.lease,retry_at=clock_timestamp()+interval '20 minutes',
            updated_at=clock_timestamp(),status='running',diagnostic=''
        WHERE knowledge_model_attempts.attempts<3 AND knowledge_model_attempts.lease<>excluded.lease
              AND knowledge_model_attempts.retry_at<=clock_timestamp()
        RETURNING attempts`, binding.ProjectID, binding.Revision, binding.ModelRevision, kind, job, lease).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// API diagnostics contain only locally defined messages, never the response,
// model input or credentials. Ignore a late result from an earlier lease.
func (s *Store) FinishKnowledgeModelAttempt(ctx context.Context, b KnowledgeRoleModel, kind string, job int64, lease, diagnostic string) error {
	target, ok := knowledgeClaimTargets[kind]
	if !ok || target.role != b.Role {
		return ErrConflict
	}
	// Completing a failed API turn releases its budget slot immediately. Its
	// persisted per-job retry time still prevents reclaiming this failed input.
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`WITH locked_job AS MATERIALIZED (
		SELECT id FROM %s WHERE project_id=$1 AND id=$5 FOR UPDATE), settled AS (
		UPDATE knowledge_model_attempts SET status=CASE WHEN $7='' THEN 'succeeded' ELSE 'failed' END,
          updated_at=clock_timestamp(),diagnostic=$7 FROM locked_job WHERE project_id=$1 AND binding_revision=$2 AND model_revision=$3
          AND kind=$4 AND job_id=locked_job.id AND lease=$6 AND status='running' RETURNING project_id,job_id,lease)
		UPDATE %s job SET %s=clock_timestamp() FROM settled s WHERE $7<>'' AND job.project_id=s.project_id
		AND job.id=s.job_id AND job.%s=s.lease AND job.%s IS NULL`, target.table, target.table, target.until, target.lease, target.result),
		b.ProjectID, b.Revision, b.ModelRevision, kind, job, lease, diagnostic)
	return err
}
