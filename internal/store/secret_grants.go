package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type SecretGrantInput struct {
	ExecutionID int64     `json:"execution_id"`
	Principal   string    `json:"principal"`
	Environment string    `json:"environment"`
	Name        string    `json:"name"`
	Version     int       `json:"version"`
	Purpose     string    `json:"purpose"`
	ExpiresAt   time.Time `json:"expires_at"`
	MaxUses     *int      `json:"max_uses"`
	RequestKey  string    `json:"request_key"`
}

func (g SecretGrantInput) Valid() bool {
	return g.ExecutionID > 0 && ValidEnvironment(g.Environment) && ValidSecretName(g.Name) && g.Version > 0 &&
		g.Principal != "" && len(g.Principal) <= 128 && strings.TrimSpace(g.Purpose) != "" && len(g.Purpose) <= 256 &&
		!strings.ContainsRune(g.Purpose, 0) && ValidSecretName(g.RequestKey) && !g.ExpiresAt.IsZero() &&
		(g.MaxUses == nil || (*g.MaxUses > 0 && *g.MaxUses <= 2147483647))
}

func (s *Store) ApproveSecretGrant(ctx context.Context, m SecretMeta, g SecretGrantInput) (json.RawMessage, error) {
	if !g.Valid() || m.Environment != g.Environment || m.Name != g.Name || m.Environment == "production" || m.Risk == "S3" {
		return nil, ErrConflict
	}
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `INSERT INTO secret_grants AS g(project_id,task_id,execution_id,principal,
		environment,name,risk,metadata_updated_at,version,purpose,expires_at,max_uses,request_key)
		SELECT t.project_id,t.id,e.id,e.worker,m.environment,m.name,m.risk,m.updated_at,$7,$8,$9,$10,$11
		FROM executions e JOIN work_items t ON t.id=e.task_id JOIN project_secret_metadata m ON m.project_id=t.project_id
		WHERE t.project_id=$1 AND e.id=$2 AND e.worker=$3 AND m.environment=$4 AND m.name=$5 AND m.updated_at=$6
		AND m.risk IN ('S0','S1','S2') AND e.result IS NULL AND e.abandoned_at IS NULL AND t.status='RUNNING' AND t.source_closure_id IS NULL AND e.owner_cancel_requested_at IS NULL
		AND least(e.lease_until,$9::timestamptz)>clock_timestamp()
		ON CONFLICT (execution_id,request_key) DO UPDATE SET request_key=EXCLUDED.request_key
		WHERE (g.project_id,g.principal,g.environment,g.name,g.metadata_updated_at,g.version,g.purpose,g.expires_at,g.max_uses)
		IS NOT DISTINCT FROM (EXCLUDED.project_id,EXCLUDED.principal,EXCLUDED.environment,EXCLUDED.name,EXCLUDED.metadata_updated_at,EXCLUDED.version,EXCLUDED.purpose,EXCLUDED.expires_at,EXCLUDED.max_uses)
		RETURNING to_jsonb(g)`, m.ProjectID, g.ExecutionID, g.Principal, g.Environment, g.Name, m.UpdatedAt, g.Version, g.Purpose, g.ExpiresAt, g.MaxUses, g.RequestKey).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	return doc, err
}

func (s *Store) SecretGrantReceipt(ctx context.Context, project, id int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(g) || jsonb_build_object('uses',COALESCE(
		(SELECT jsonb_agg(to_jsonb(u) ORDER BY u.id) FROM secret_grant_uses u WHERE u.grant_id=g.id),'[]'))
		FROM secret_grants g WHERE project_id=$1 AND id=$2`, project, id).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) RevokeSecretGrant(ctx context.Context, project, id int64) error {
	var found int64
	err := s.db.QueryRowContext(ctx, `UPDATE secret_grants SET revoked_at=COALESCE(revoked_at,clock_timestamp())
		WHERE project_id=$1 AND id=$2 RETURNING id`, project, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

type SecretUse struct {
	ExecutionID int64  `json:"execution_id"`
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Purpose     string `json:"purpose"`
	RequestKey  string `json:"request_key"`
	Lease       string `json:"lease_token"`
}

// The caller releases the one-use wrapper only after this transaction commits.
func (s *Store) UseSecret(ctx context.Context, project, id int64, principal string, u SecretUse,
	wrap func(SecretMeta, int, time.Time) (time.Time, error)) error {
	if !ValidSecretName(u.RequestKey) || !ValidSecretName(u.Name) || !ValidEnvironment(u.Environment) || u.Environment == "production" || len(u.Lease) != 64 || u.ExecutionID < 1 {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	m := SecretMeta{ProjectID: project, Environment: u.Environment, Name: u.Name}
	var hash []byte
	var version int
	var limit sql.NullInt64
	var until time.Time
	err = tx.QueryRowContext(ctx, `SELECT e.lease_hash,g.version,g.max_uses,least(e.lease_until,g.expires_at),m.risk
		FROM secret_grants g JOIN executions e ON e.id=g.execution_id JOIN work_items t ON t.id=e.task_id
		JOIN project_secret_metadata m ON (m.project_id,m.environment,m.name)=(g.project_id,g.environment,g.name)
		WHERE g.project_id=$1 AND g.id=$2 AND e.worker=$3 AND g.principal=$3 AND g.execution_id=$4
		AND g.environment=$5 AND g.name=$6 AND g.purpose=$7 AND g.revoked_at IS NULL AND g.risk=m.risk AND g.metadata_updated_at=m.updated_at
		AND t.project_id=g.project_id AND t.status='RUNNING' AND t.source_closure_id IS NULL AND e.owner_cancel_requested_at IS NULL AND e.result IS NULL AND e.abandoned_at IS NULL
		AND least(e.lease_until,g.expires_at)>clock_timestamp() FOR UPDATE OF g FOR SHARE OF e,t,m`,
		project, id, principal, u.ExecutionID, u.Environment, u.Name, u.Purpose).Scan(&hash, &version, &limit, &until, &m.Risk)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(u.Lease))
	if subtle.ConstantTimeCompare(hash, sum[:]) != 1 {
		return ErrConflict
	}
	var issued int64
	var repeated bool
	err = tx.QueryRowContext(ctx, `SELECT count(*) FILTER(WHERE outcome='ISSUED'),COALESCE(bool_or(request_key=$2),false)
		FROM secret_grant_uses WHERE grant_id=$1`, id, u.RequestKey).Scan(&issued, &repeated)
	if err != nil {
		return err
	}
	if repeated || (limit.Valid && issued >= limit.Int64) {
		return ErrConflict
	}
	expires, issueErr := wrap(m, version, until)
	outcome := "FAILED"
	var deadline *time.Time
	if issueErr == nil {
		if expires.After(until) || !expires.After(time.Now()) {
			issueErr = ErrConflict
		} else {
			outcome = "ISSUED"
			deadline = &expires
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO secret_grant_uses(grant_id,request_key,outcome,wrapper_expires_at) VALUES ($1,$2,$3,$4)`, id, u.RequestKey, outcome, deadline)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return issueErr
}
