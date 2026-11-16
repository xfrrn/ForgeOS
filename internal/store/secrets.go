package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"
)

type SecretMeta struct {
	ProjectID   int64     `json:"project_id"`
	Environment string    `json:"environment"`
	Name        string    `json:"name"`
	Risk        string    `json:"risk"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var secretName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,95}$`)

func ValidSecretName(v string) bool { return secretName.MatchString(v) }
func ValidEnvironment(v string) bool {
	switch v {
	case "local-mock", "local-integrated", "ci-test", "staging", "production":
		return true
	}
	return false
}
func ValidRisk(v string) bool { return v == "S0" || v == "S1" || v == "S2" || v == "S3" }

func (s *Store) SetSecretMeta(ctx context.Context, m SecretMeta) (SecretMeta, error) {
	if !ValidSecretName(m.Name) || !ValidEnvironment(m.Environment) || !ValidRisk(m.Risk) {
		return m, ErrConflict
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO project_secret_metadata AS m(project_id,environment,name,risk)
		SELECT id,$2,$3,$4 FROM projects WHERE id=$1 ON CONFLICT (project_id,environment,name)
		DO UPDATE SET risk=EXCLUDED.risk,updated_at=CASE WHEN m.risk=EXCLUDED.risk THEN m.updated_at ELSE now() END
		RETURNING updated_at`, m.ProjectID, m.Environment, m.Name, m.Risk).Scan(&m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}
func (s *Store) SecretMeta(ctx context.Context, project int64, environment, name string) (SecretMeta, error) {
	m := SecretMeta{ProjectID: project, Environment: environment, Name: name}
	err := s.db.QueryRowContext(ctx, `SELECT risk,updated_at FROM project_secret_metadata WHERE project_id=$1 AND environment=$2 AND name=$3`, project, environment, name).Scan(&m.Risk, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}
