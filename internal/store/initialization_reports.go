package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// The inventory is already immutable. Store only the evidence that can change after the HEAD check.
const onboardingSnapshot = `SELECT jsonb_build_object(
	'repository_mapping',jsonb_build_object('project_id',p.id,'path',p.path,'github_origin',i.github_origin,'repository_id',i.repository_id),
	'project_profile',jsonb_build_object('status',CASE WHEN a.task_id IS NULL THEN 'NOT_REQUESTED' WHEN a.result IS NOT NULL THEN 'DONE'
		WHEN a.failure IS NOT NULL THEN 'FAILED' WHEN a.lease_until>now() THEN 'RUNNING' ELSE 'READY' END,
		'attempt',a.attempt,'failure',a.failure,'analysis',a.result,'finished_at',a.finished_at,'omitted',a.source->'omitted','truncated',a.source->'truncated',
		'sources',COALESCE((SELECT jsonb_agg(d-'text' ORDER BY pos) FROM jsonb_array_elements(a.source->'sources') WITH ORDINALITY docs(d,pos)),'[]'::jsonb)),
	'materials',COALESCE((SELECT jsonb_agg(to_jsonb(m) ORDER BY key) FROM (
		SELECT DISTINCT ON (key) key,revision,kind,title,content_sha256 FROM project_materials WHERE project_id=i.project_id
		ORDER BY key,revision DESC) m WHERE m.kind IN ('profile','environment')),'[]'::jsonb),
	'startup_checks',COALESCE((SELECT jsonb_agg(jsonb_build_object('task_id',c.task_id,'environment',c.environment,'status',t.status,
		'execution_id',e.id,'worker',e.worker,'error',e.result->>'error','receipt',e.result->'environment_run') ORDER BY c.task_id DESC)
		FROM startup_checks c JOIN work_items t ON t.id=c.task_id
		LEFT JOIN LATERAL(SELECT id,worker,result FROM executions WHERE task_id=t.id ORDER BY id DESC LIMIT 1) e ON true
		WHERE c.source_task_id=i.task_id),'[]'::jsonb))
	FROM repository_inventories i JOIN projects p ON p.id=i.project_id LEFT JOIN repository_analyses a USING(task_id) WHERE i.task_id=$1`

func (s *Store) CaptureInitializationReport(ctx context.Context, task int64) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var locked int64
	err = tx.QueryRowContext(ctx, `SELECT task_id FROM repository_inventories WHERE task_id=$1 FOR UPDATE`, task).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var snapshot []byte
	if err = tx.QueryRowContext(ctx, onboardingSnapshot, task).Scan(&snapshot); err != nil {
		return nil, err
	}
	var revision int
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT revision,snapshot=$2::jsonb FROM initialization_reports WHERE task_id=$1 ORDER BY revision DESC LIMIT 1`, task, string(snapshot)).Scan(&revision, &same)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if !same {
		revision++
		if _, err = tx.ExecContext(ctx, `INSERT INTO initialization_reports(task_id,revision,snapshot) VALUES($1,$2,$3::jsonb)`, task, revision, string(snapshot)); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.InitializationReport(ctx, task, revision)
}

func (s *Store) InitializationReport(ctx context.Context, task int64, revision int) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT (to_jsonb(r)-'snapshot')||r.snapshot||jsonb_build_object('repository_inventory',to_jsonb(i))
		FROM initialization_reports r JOIN repository_inventories i USING(task_id) WHERE r.task_id=$1 AND r.revision=$2`, task, revision).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}

func (s *Store) InitializationReports(ctx context.Context, task int64) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT jsonb_agg(jsonb_build_object('revision',revision,'generated_at',generated_at) ORDER BY revision DESC)
		FROM initialization_reports WHERE task_id=i.task_id),'[]'::jsonb) FROM repository_inventories i WHERE task_id=$1`, task).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}
