package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/lib/pq"
)

type RepositoryCredential struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	URL      string  `json:"url"`
	Username string  `json:"username"`
	Enabled  bool    `json:"enabled"`
	Version  int     `json:"version"`
	VaultKey string  `json:"-"`
	Projects []int64 `json:"projects,omitempty"`
}

const repositoryCredentialColumns = `id,name,url,username,enabled,version,vault_key`

func (s *Store) RepositoryCredentials(ctx context.Context) ([]RepositoryCredential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+repositoryCredentialColumns+`,
		ARRAY(SELECT p.id FROM projects p WHERE p.repository_credential_id=c.id ORDER BY p.id)
		FROM repository_credentials c ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RepositoryCredential{}
	for rows.Next() {
		var c RepositoryCredential
		if err := rows.Scan(&c.ID, &c.Name, &c.URL, &c.Username, &c.Enabled, &c.Version, &c.VaultKey, pq.Array(&c.Projects)); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (s *Store) RepositoryCredential(ctx context.Context, id int64) (RepositoryCredential, error) {
	var c RepositoryCredential
	err := s.db.QueryRowContext(ctx, `SELECT `+repositoryCredentialColumns+` FROM repository_credentials WHERE id=$1`, id).
		Scan(&c.ID, &c.Name, &c.URL, &c.Username, &c.Enabled, &c.Version, &c.VaultKey)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return c, err
}

func (s *Store) ReserveRepositoryCredential(ctx context.Context, c RepositoryCredential) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO repository_credentials(name,url,username) VALUES($1,$2,$3)
		ON CONFLICT(url,name) DO NOTHING RETURNING id`, c.Name, c.URL, c.Username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrConflict
	}
	return id, err
}

// Pin the exact KV version only after the vault acknowledges the write. A failed
// rotation leaves the previous version usable, including after an ambiguous timeout.
func (s *Store) RotateRepositoryCredential(ctx context.Context, id int64, write func(string) (int, error)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var key string
	err = tx.QueryRowContext(ctx, `SELECT vault_key FROM repository_credentials WHERE id=$1 FOR UPDATE`, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	version, err := write(key)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE repository_credentials SET version=$2,enabled=true WHERE id=$1`, id, version); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DisableRepositoryCredential(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE repository_credentials SET enabled=false WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if n == 0 && err == nil {
		return ErrNotFound
	}
	return err
}

func (s *Store) SetProjectRepositoryIdentity(ctx context.Context, project, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE projects SET repository_credential_id=NULLIF($2::bigint,0) WHERE id=$1`, project, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if n == 0 && err == nil {
		return ErrNotFound
	}
	return err
}
