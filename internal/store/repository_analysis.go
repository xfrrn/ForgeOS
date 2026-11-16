package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"forgeos/internal/localrepo"
	"github.com/lib/pq"
)

type AnalysisJob struct {
	Task  int64                   `json:"task_id"`
	Lease string                  `json:"lease"`
	Input localrepo.AnalysisInput `json:"input"`
}

type AnalysisFact struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Path  string `json:"path"`
	Quote string `json:"quote"`
}
type RepositoryAnalysis struct {
	Session string         `json:"session_id"`
	Facts   []AnalysisFact `json:"facts"`
}
type AnalysisResult struct {
	Task     int64               `json:"task_id"`
	Lease    string              `json:"lease"`
	Analysis *RepositoryAnalysis `json:"analysis,omitempty"`
	Failure  string              `json:"failure,omitempty"`
}

func (a RepositoryAnalysis) Valid(input localrepo.AnalysisInput) bool {
	if a.Session == "" || len(a.Session) > 128 || strings.Trim(a.Session, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" || a.Facts == nil || len(a.Facts) > 40 {
		return false
	}
	for _, f := range a.Facts {
		if !slices.Contains([]string{"profile", "stack", "dependency", "start", "test"}, f.Kind) || strings.TrimSpace(f.Value) == "" || strings.ContainsRune(f.Value, 0) || len(f.Value) > 500 || strings.TrimSpace(f.Quote) == "" || len(f.Quote) > 768 {
			return false
		}
		if (f.Kind == "start" || f.Kind == "test") && !strings.Contains(f.Quote, f.Value) {
			return false
		}
		if !slices.ContainsFunc(input.Sources, func(s localrepo.AnalysisSource) bool { return s.Path == f.Path && strings.Contains(s.Text, f.Quote) }) {
			return false
		}
	}
	return true
}

func (s *Store) RepositoryAnalysis(ctx context.Context, id int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT jsonb_build_object('status',CASE WHEN result IS NOT NULL THEN 'DONE' WHEN failure IS NOT NULL THEN 'FAILED'
		WHEN lease_until>now() THEN 'RUNNING' ELSE 'READY' END,'attempt',attempt,'analysis',result,'failure',failure,'finished_at',finished_at,
		'omitted',source->'omitted','truncated',source->'truncated','sources',COALESCE((SELECT jsonb_agg(d-'text') FROM jsonb_array_elements(source->'sources') d),'[]'::jsonb))
		FROM repository_analyses WHERE task_id=$1`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}

func (s *Store) QueueAnalysis(ctx context.Context, id int64, input localrepo.AnalysisInput) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO repository_analyses(task_id,source) VALUES($1,$2::jsonb) ON CONFLICT DO NOTHING`, id, string(raw))
	return err
}

func (s *Store) RetryAnalysis(ctx context.Context, id int64, attempt int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE repository_analyses SET failure=NULL,finished_at=NULL,lease_until=NULL,lease=NULL
		WHERE task_id=$1 AND attempt=$2 AND failure IS NOT NULL`, id, attempt)
	return err
}

func (s *Store) ClaimAnalysis(ctx context.Context, project int64) (*AnalysisJob, error) {
	var j AnalysisJob
	var raw []byte
	err := s.db.QueryRowContext(ctx, `WITH next AS (SELECT a.task_id FROM repository_analyses a JOIN repository_inventories i USING(task_id)
		WHERE i.project_id=$1 AND a.result IS NULL AND a.failure IS NULL AND (a.lease_until IS NULL OR a.lease_until<=now())
		ORDER BY a.task_id LIMIT 1 FOR UPDATE OF a SKIP LOCKED)
		UPDATE repository_analyses a SET lease=gen_random_uuid(),lease_until=now()+interval '6 minutes',attempt=attempt+1 FROM next
		WHERE a.task_id=next.task_id RETURNING a.task_id,a.lease,a.source`, project).Scan(&j.Task, &j.Lease, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, json.Unmarshal(raw, &j.Input)
}

func (s *Store) FinishAnalysis(ctx context.Context, project int64, r AnalysisResult) error {
	if r.Task < 1 || r.Lease == "" || (r.Analysis == nil && r.Failure != "agent_run_failed" && r.Failure != "unverified_evidence") || (r.Analysis != nil && r.Failure != "") {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw []byte
	var finished, live, same bool
	var report any
	if r.Analysis != nil {
		b, err := json.Marshal(r.Analysis)
		if err != nil {
			return err
		}
		report = string(b)
	}
	err = tx.QueryRowContext(ctx, `SELECT source,finished_at IS NOT NULL,lease_until>clock_timestamp(),
		result IS NOT DISTINCT FROM $4::jsonb AND failure IS NOT DISTINCT FROM NULLIF($5,'')
		FROM repository_analyses a JOIN repository_inventories i USING(task_id)
		WHERE task_id=$1 AND i.project_id=$2 AND lease::text=$3 FOR UPDATE OF a`, r.Task, project, r.Lease, report, r.Failure).Scan(&raw, &finished, &live, &same)
	var pg *pq.Error
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &pg) && pg.Code == "22P05") {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if finished && same {
		return nil
	}
	if finished || !live {
		return ErrConflict
	}
	var input localrepo.AnalysisInput
	if err = json.Unmarshal(raw, &input); err != nil {
		return err
	}
	if r.Analysis != nil && !r.Analysis.Valid(input) {
		return ErrConflict
	}
	res, err := tx.ExecContext(ctx, `UPDATE repository_analyses SET result=$2::jsonb,failure=NULLIF($3,''),finished_at=clock_timestamp()
		WHERE task_id=$1 AND lease_until>clock_timestamp()`, r.Task, report, r.Failure)
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return tx.Commit()
}
