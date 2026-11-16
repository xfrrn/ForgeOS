package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"

	"forgeos/internal/validation"
)

var ErrCIExpired = errors.New("CI 文件已按项目保留策略到期")

type CIRetention struct {
	LogSeconds      int32 `json:"log_seconds"`
	ReportSeconds   int32 `json:"report_seconds"`
	ArtifactSeconds int32 `json:"artifact_seconds"`
}

const ciDue = `a.confirmed_at + make_interval(secs => NULLIF((p.ci_retention->>(a.kind || '_seconds'))::integer,0)) <= statement_timestamp()`

const markCIExpired = `UPDATE ci_archive_files a SET expired_at=statement_timestamp(),retention_revision=p.ci_revision
	FROM executions e JOIN work_items t ON t.id=e.task_id JOIN projects p ON p.id=t.project_id
	WHERE a.execution_id=e.id AND p.ci_retention <> '{}'::jsonb AND a.expired_at IS NULL AND ` + ciDue

func (s *Store) ProjectCIRetention(ctx context.Context, id int64, policy *CIRetention) (CIRetention, error) {
	return s.projectCIRetention(ctx, id, policy, false)
}

func (s *Store) ProjectLocalCIRetention(ctx context.Context, id int64, policy *CIRetention) (CIRetention, error) {
	return s.projectCIRetention(ctx, id, policy, true)
}

const markLocalCIExpired = `UPDATE ci_archive_files a SET local_expired_at=statement_timestamp(),local_revision=p.local_ci_revision
    FROM executions e JOIN work_items t ON t.id=e.task_id JOIN projects p ON p.id=t.project_id
    WHERE a.execution_id=e.id AND p.local_ci_retention <> '{}'::jsonb AND a.local_expired_at IS NULL
    AND a.confirmed_at + make_interval(secs => NULLIF((p.local_ci_retention->>(a.kind || '_seconds'))::integer,0)) <= statement_timestamp()`

func (s *Store) projectCIRetention(ctx context.Context, id int64, policy *CIRetention, local bool) (CIRetention, error) {
	column, rev, scope, expire := "ci_retention", "ci_revision", "ci", markCIExpired
	if local {
		column, rev, scope, expire = "local_ci_retention", "local_ci_revision", "local_ci", markLocalCIExpired
	}
	var current CIRetention
	var raw []byte
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return current, err
	}
	defer tx.Rollback()
	query := "SELECT " + column + " FROM projects WHERE id=$1"
	if policy != nil {
		query += " FOR UPDATE"
	}
	err = tx.QueryRowContext(ctx, query, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return current, ErrNotFound
	}
	if err != nil {
		return current, err
	}
	if policy == nil {
		err = json.Unmarshal(raw, &current)
		return current, err
	}
	if policy.LogSeconds < 0 || policy.ReportSeconds < 0 || policy.ArtifactSeconds < 0 {
		return current, ErrConflict
	}
	// A longer replacement policy must not revive files already due under the old one.
	if _, err = tx.ExecContext(ctx, expire+" AND p.id=$1", id); err != nil {
		return current, err
	}
	next, err := json.Marshal(policy)
	if err != nil {
		return current, err
	}
	if *policy == (CIRetention{}) {
		next = []byte("{}")
	}
	revision, err := recordRetentionChange(ctx, tx, id, scope, raw, next)
	if err != nil {
		return current, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE projects SET "+column+"=$2,"+rev+"=COALESCE($3,"+rev+") WHERE id=$1", id, string(next), revision); err != nil {
		return current, err
	}
	if _, err = tx.ExecContext(ctx, expire+" AND p.id=$1", id); err != nil {
		return current, err
	}
	return *policy, tx.Commit()
}

// Hold the file row lock until I/O finishes so a late upload cannot undo a purge.
func (s *Store) AccessCIFile(ctx context.Context, task, execution int64, runID, file string, access func(validation.FileDigest) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw []byte
	var expired, due bool
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT e.result->'validation'->'run',a.expired_at IS NOT NULL,COALESCE(`+ciDue+`,false),
		COALESCE(CASE WHEN a.expired_at IS NOT NULL THEN a.retention_revision ELSE p.ci_revision END,0)
		FROM ci_archive_files a JOIN executions e ON e.id=a.execution_id
		JOIN work_items t ON t.id=e.task_id JOIN projects p ON p.id=t.project_id
		WHERE e.id=$1 AND e.task_id=$2 AND e.result->'validation'->'run'->>'id'=$3 AND a.file=$4
		FOR UPDATE OF a`, execution, task, runID, file).Scan(&raw, &expired, &due, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if expired {
		return RetentionExpiry{ErrCIExpired, revision}
	}
	if due {
		if _, err = tx.ExecContext(ctx, "UPDATE ci_archive_files SET expired_at=now(),retention_revision=NULLIF($3::bigint,0) WHERE execution_id=$1 AND file=$2", execution, file, revision); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return RetentionExpiry{ErrCIExpired, revision}
	}
	var run validation.Run
	if err = json.Unmarshal(raw, &run); err != nil {
		return err
	}
	if !run.Valid() || run.ExecutionID != execution {
		return ErrNotFound
	}
	digest, ok := run.Files()[file]
	if !ok {
		return ErrNotFound
	}
	if err = access(digest); err != nil {
		return err
	}
	return tx.Commit()
}

type CIArchiveFile struct {
	Task, Execution int64
	Run, File       string
}

func (s *Store) PurgeCIFiles(ctx context.Context, remove func(CIArchiveFile) error) error {
	if _, err := s.db.ExecContext(ctx, markCIExpired); err != nil {
		return err
	}
	var after CIArchiveFile
	var firstError error
	for {
		rows, err := s.db.QueryContext(ctx, `SELECT e.task_id,e.id,e.result->'validation'->'run'->>'id',a.file
			FROM ci_archive_files a JOIN executions e ON e.id=a.execution_id
			WHERE a.expired_at IS NOT NULL AND a.removed_at IS NULL AND (a.execution_id,a.file)>($1,$2)
			ORDER BY a.execution_id,a.file LIMIT 128`, after.Execution, after.File)
		if err != nil {
			return err
		}
		var batch []CIArchiveFile
		for rows.Next() {
			var f CIArchiveFile
			if err = rows.Scan(&f.Task, &f.Execution, &f.Run, &f.File); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, f)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			return firstError
		}
		for _, f := range batch {
			if err = remove(f); err != nil {
				if firstError == nil {
					firstError = err
				}
				continue
			}
			if _, err = s.db.ExecContext(ctx, "UPDATE ci_archive_files SET removed_at=now() WHERE execution_id=$1 AND file=$2 AND removed_at IS NULL", f.Execution, f.File); err != nil {
				return err
			}
		}
		after = batch[len(batch)-1]
	}
}

type LocalCIFile struct {
	Removed   bool                  `json:"removed,omitempty"`
	Task      int64                 `json:"task"`
	Execution int64                 `json:"execution"`
	Run       string                `json:"run"`
	File      string                `json:"file"`
	Revision  int64                 `json:"revision"`
	Digest    validation.FileDigest `json:"digest"`
}

func (s *Store) ExpireLocalCIFiles(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, markLocalCIExpired)
	return err
}

func (s *Store) LocalCICleanup(ctx context.Context, worker string, projects []int64, after int64, file string, reconcile bool) ([]LocalCIFile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.task_id,e.id,a.file,a.local_revision,e.result->'validation'->'run',a.local_removed_at IS NOT NULL
        FROM ci_archive_files a JOIN executions e ON e.id=a.execution_id JOIN work_items t ON t.id=e.task_id
        WHERE a.local_expired_at IS NOT NULL AND ($5 OR a.local_removed_at IS NULL) AND e.worker=$1
        AND ($2::bigint[] IS NULL OR t.project_id=ANY($2)) AND (e.id,a.file)>($3,$4)
        ORDER BY e.id,a.file LIMIT 32`, worker, pq.Array(projects), after, file, reconcile)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []LocalCIFile{}
	for rows.Next() {
		var f LocalCIFile
		var raw []byte
		if err = rows.Scan(&f.Task, &f.Execution, &f.File, &f.Revision, &raw, &f.Removed); err != nil {
			return nil, err
		}
		var run validation.Run
		if err = json.Unmarshal(raw, &run); err != nil {
			return nil, err
		}
		if !run.Valid() || run.Runner != worker || run.ExecutionID != f.Execution {
			return nil, errors.New("invalid archived Worker run")
		}
		digest, ok := run.Files()[f.File]
		if !ok {
			return nil, errors.New("file missing from CI receipt")
		}
		f.Run, f.Digest = run.ID, digest
		list = append(list, f)
	}
	return list, rows.Err()
}

func (s *Store) ConfirmLocalCIRemoval(ctx context.Context, worker string, projects []int64, execution int64, file string, revision int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE ci_archive_files a SET local_removed_at=COALESCE(a.local_removed_at,now())
        FROM executions e JOIN work_items t ON t.id=e.task_id WHERE e.id=a.execution_id AND e.worker=$1
        AND ($2::bigint[] IS NULL OR t.project_id=ANY($2)) AND e.id=$3 AND a.file=$4
        AND a.local_expired_at IS NOT NULL AND a.local_revision=$5`, worker, pq.Array(projects), execution, file, revision)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}
