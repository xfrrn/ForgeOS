package store

import (
	"context"
	"database/sql"
	"errors"
)

type RepositoryClone struct {
	ID           int64
	CredentialID int64
	ProjectID    int64
	URL          string
	Repository   string
	Directory    string
	Lease        string `json:"-"`
	Status       string
	Path         string
	Error        string
}

func (s *Store) CloneDirectory(ctx context.Context) (string, error) {
	var directory string
	err := s.db.QueryRowContext(ctx, "SELECT directory FROM project_clone_settings WHERE id=1").Scan(&directory)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return directory, err
}

func (s *Store) SetCloneDirectory(ctx context.Context, directory string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO project_clone_settings(id,directory) VALUES (1,$1)
		ON CONFLICT(id) DO UPDATE SET directory=EXCLUDED.directory`, directory)
	return err
}

const cloneColumns = `id,COALESCE(project_id,0),url,repository,directory,COALESCE(lease::text,''),status,path,error,COALESCE(credential_id,0)`

func scanRepositoryClone(row interface{ Scan(...any) error }) (RepositoryClone, error) {
	var j RepositoryClone
	err := row.Scan(&j.ID, &j.ProjectID, &j.URL, &j.Repository, &j.Directory, &j.Lease, &j.Status, &j.Path, &j.Error, &j.CredentialID)
	if errors.Is(err, sql.ErrNoRows) {
		return RepositoryClone{}, ErrNotFound
	}
	return j, err
}

func (s *Store) QueueRepositoryClone(ctx context.Context, url, repository, directory string, credentialID int64) (RepositoryClone, error) {
	j, err := scanRepositoryClone(s.db.QueryRowContext(ctx, `INSERT INTO repository_clone_jobs(url,repository,directory,credential_id)
		VALUES ($1,$2,$3,NULLIF($4::bigint,0)) ON CONFLICT(url) DO UPDATE SET
			credential_id=EXCLUDED.credential_id,repository=EXCLUDED.repository,directory=EXCLUDED.directory,status='queued',error='',updated_at=now()
		WHERE repository_clone_jobs.status='failed'
		RETURNING `+cloneColumns, url, repository, directory, credentialID))
	if errors.Is(err, ErrNotFound) {
		j, err = scanRepositoryClone(s.db.QueryRowContext(ctx, `SELECT `+cloneColumns+` FROM repository_clone_jobs WHERE url=$1`, url))
		if err == nil && j.Status != "done" && j.CredentialID != credentialID {
			return RepositoryClone{}, ErrConflict
		}
	}
	return j, err
}

func (s *Store) RepositoryClone(ctx context.Context, id int64) (RepositoryClone, error) {
	return scanRepositoryClone(s.db.QueryRowContext(ctx, `SELECT `+cloneColumns+` FROM repository_clone_jobs WHERE id=$1`, id))
}

func (s *Store) ClaimRepositoryClone(ctx context.Context) (*RepositoryClone, error) {
	job, err := scanRepositoryClone(s.db.QueryRowContext(ctx, `WITH next_job AS (
		SELECT id FROM repository_clone_jobs
		WHERE status='queued' OR (status='cloning' AND lease_until<=clock_timestamp())
		ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED
	)
	UPDATE repository_clone_jobs SET status='cloning',lease=gen_random_uuid(),
		lease_until=clock_timestamp()+interval '13 minutes',updated_at=now()
	WHERE id=(SELECT id FROM next_job)
	RETURNING `+cloneColumns))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

const activeCloneLease = `id=$1 AND lease::text=$2 AND status='cloning' AND lease_until>clock_timestamp()`

func (s *Store) CompleteRepositoryClone(ctx context.Context, id int64, lease, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM repository_clone_jobs WHERE `+activeCloneLease+` FOR UPDATE`, id, lease).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	project, err := createProject(ctx, tx, path)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE projects SET repository_credential_id=(SELECT credential_id FROM repository_clone_jobs WHERE id=$1) WHERE id=$2`, id, project.ID); err != nil {
		return err
	}
	if err = settleRepositoryClone(tx.ExecContext(ctx, `UPDATE repository_clone_jobs SET status='done',
		project_id=$3,path=$4,lease=NULL,lease_until=NULL,updated_at=now() WHERE `+activeCloneLease,
		id, lease, project.ID, project.Path)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailRepositoryClone(ctx context.Context, id int64, lease, message string) error {
	return settleRepositoryClone(s.db.ExecContext(ctx, `UPDATE repository_clone_jobs SET status='failed',
		error=$3,lease=NULL,lease_until=NULL,updated_at=now() WHERE `+activeCloneLease, id, lease, message))
}

func (s *Store) RequeueRepositoryClone(ctx context.Context, id int64, lease string) error {
	return settleRepositoryClone(s.db.ExecContext(ctx, `UPDATE repository_clone_jobs SET status='queued',
		lease=NULL,lease_until=NULL,updated_at=now() WHERE `+activeCloneLease, id, lease))
}

func settleRepositoryClone(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
