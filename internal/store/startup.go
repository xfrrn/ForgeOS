package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"forgeos/internal/validation"
	"forgeos/schemas"
)

type EnvironmentRequest struct {
	Repo        string `json:"repo"`
	Commit      string `json:"commit"`
	Environment string `json:"environment"`
}

type EnvironmentRun struct {
	EnvironmentRequest
	ID         string                    `json:"id"`
	Worker     string                    `json:"worker"`
	Started    time.Time                 `json:"started_at"`
	Finished   time.Time                 `json:"finished_at"`
	ConfigBlob string                    `json:"config_blob,omitempty"`
	ScriptBlob string                    `json:"script_blob,omitempty"`
	Image      string                    `json:"image,omitempty"`
	Network    string                    `json:"network,omitempty"`
	Config     *schemas.LocalEnvironment `json:"config,omitempty"`
	Probes     int                       `json:"probes"`
	StatusCode int                       `json:"http_status,omitempty"`
	Removed    bool                      `json:"container_removed,omitempty"`
	Failure    string                    `json:"failure,omitempty"`
}

type StartupSpec struct {
	Source      int64                    `json:"source_task_id"`
	Environment string                   `json:"environment"`
	Commit      string                   `json:"commit"`
	ConfigBlob  string                   `json:"config_blob"`
	Config      schemas.LocalEnvironment `json:"config"`
}

func startupSpec(ctx context.Context, tx interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (*StartupSpec, error) {
	var spec StartupSpec
	var raw []byte
	err := tx.QueryRowContext(ctx, `SELECT i.task_id,c.environment,i.commit_sha,f->>'blob',f->'local_environments'->c.environment
        FROM startup_checks c JOIN repository_inventories i ON i.task_id=c.source_task_id
        CROSS JOIN LATERAL jsonb_array_elements(i.files) f WHERE c.task_id=$1 AND f->>'path'='project.yaml'`, id).Scan(&spec.Source, &spec.Environment, &spec.Commit, &spec.ConfigBlob, &raw)
	if err != nil {
		return nil, err
	}
	return &spec, json.Unmarshal(raw, &spec.Config)
}

func (s *Store) QueueStartup(ctx context.Context, source int64, env string, previous int64) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var project int64
	var declared bool
	err = tx.QueryRowContext(ctx, `SELECT project_id, EXISTS(SELECT 1 FROM jsonb_array_elements(files) f
        WHERE f->>'path'='project.yaml' AND (f->'local_environments') ? $2)
        FROM repository_inventories WHERE task_id=$1 FOR UPDATE`, source, env).Scan(&project, &declared)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if !declared {
		return 0, ErrConflict
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT task_id FROM startup_checks
        WHERE source_task_id=$1 AND environment=$2 ORDER BY task_id DESC LIMIT 1`, source, env).Scan(&id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if previous != 0 {
		var belongs bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM startup_checks c JOIN work_items t ON t.id=c.task_id
			WHERE c.task_id=$1 AND c.source_task_id=$2 AND c.environment=$3 AND t.status IN ('DONE','FAILED'))`, previous, source, env).Scan(&belongs)
		if err != nil {
			return 0, err
		}
		if !belongs {
			return 0, ErrConflict
		}
	}
	if id > 0 && previous != id {
		return id, nil
	}
	if err = tx.QueryRowContext(ctx, `INSERT INTO work_items(project_id,status,kind) VALUES($1,'READY','environment_check') RETURNING id`, project).Scan(&id); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO startup_checks(task_id,source_task_id,environment) VALUES($1,$2,$3)`, id, source, env); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type) VALUES($1,'task_created')`, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) StartupChecks(ctx context.Context, source int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT jsonb_agg(jsonb_build_object('id',c.task_id,'environment',c.environment,'status',t.status) ORDER BY c.task_id DESC)
        FROM startup_checks c JOIN work_items t ON t.id=c.task_id WHERE c.source_task_id=i.task_id),'[]'::jsonb)
        FROM repository_inventories i WHERE i.task_id=$1`, source).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}

var dockerDigest = regexp.MustCompile("^[a-f0-9]{64}$")

func (s StartupSpec) accepts(result Result, worker, runID string) bool {
	r := result.EnvironmentRun
	if r == nil {
		return result.ExitCode != 0 && result.Error != "" && result.Commit == ""
	}
	if r.ID != runID || r.Worker != worker || r.Environment != s.Environment || r.Commit != s.Commit || result.Commit != s.Commit ||
		r.Repo == "" || r.Started.IsZero() || r.Finished.Before(r.Started) || r.Probes < 0 || (r.StatusCode != 0 && (r.StatusCode < 100 || r.StatusCode > 599)) ||
		(r.ConfigBlob != "" && r.ConfigBlob != s.ConfigBlob) || (r.Config != nil && *r.Config != s.Config) {
		return false
	}
	if result.ExitCode != 0 {
		return result.Error != "" && r.Failure != ""
	}
	return result.Error == "" && r.Failure == "" && r.Config != nil && r.ConfigBlob == s.ConfigBlob && validation.FullCommit(r.ScriptBlob) &&
		len(r.Image) == 71 && r.Image[:7] == "sha256:" && dockerDigest.MatchString(r.Image[7:]) &&
		((s.Environment == "local-mock" && r.Network == "none") || (s.Environment == "local-integrated" && dockerDigest.MatchString(r.Network))) &&
		r.Removed && r.Probes > 0 && r.StatusCode >= 200 && r.StatusCode < 300
}
