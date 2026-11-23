package store

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type FeatureClient struct {
	Status         string     `json:"status,omitempty"`
	Node           string     `json:"node,omitempty"`
	ResourceWorker string     `json:"resource_worker,omitempty"`
	CheckedAt      *time.Time `json:"checked_at,omitempty"`
	Pending        bool       `json:"pending,omitempty"`
	ID             uuid.UUID  `json:"id"`
	Instance       string     `json:"instance"`
	Backend        string     `json:"backend"`
	Model          string     `json:"model"`
}

func (s *Store) ReportFeatureClients(ctx context.Context, project int64, node string, clients []FeatureClient) error {
	return s.RecordFeatureProbe(ctx, project, node, "", clients, 0, false)
}

func (s *Store) RecordFeatureProbe(ctx context.Context, project int64, node, resource string, clients []FeatureClient, generation int64, configFailed bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if configFailed {
		if _, err = tx.ExecContext(ctx, "UPDATE feature_clients SET seen_at='-infinity',probe_status='configuration',checked_at=now() WHERE project_id=$1 AND node=$2 AND probe_status<>'removed'", project, node); err != nil {
			return err
		}
	} else if _, err = tx.ExecContext(ctx, "UPDATE feature_clients SET seen_at='-infinity',probe_status='removed' WHERE project_id=$1 AND node=$2 AND probe_status<>'removed'", project, node); err != nil {
		return err
	}
	for _, c := range clients {
		status := c.Status
		if status == "" {
			status = "ready"
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO feature_clients(project_id,node,instance,backend,model,probe_status,checked_at,seen_at,resource_worker)
        VALUES($1,$2,$3,$4,$5,$6,now(),CASE WHEN $6='ready' THEN now() ELSE '-infinity'::timestamptz END,$7)
        ON CONFLICT(project_id,node,instance,backend,model) DO UPDATE SET probe_status=excluded.probe_status,
        checked_at=excluded.checked_at,seen_at=excluded.seen_at,resource_worker=excluded.resource_worker`, project, node, c.Instance, c.Backend, c.Model, status, resource); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE feature_probe_requests SET completed=greatest(completed,least(requested,$3)) WHERE project_id=$1 AND node=$2`, project, node, generation); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) FeatureClients(ctx context.Context, project int64) ([]FeatureClient, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,instance,backend,model FROM feature_clients WHERE project_id=$1 AND seen_at>now()-interval '3 minutes' ORDER BY backend,model,instance", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeatureClient{}
	for rows.Next() {
		var c FeatureClient
		if err = rows.Scan(&c.ID, &c.Instance, &c.Backend, &c.Model); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ClientHealth(ctx context.Context, project int64) ([]FeatureClient, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.instance,c.backend,c.model,c.node,c.checked_at,
 CASE WHEN c.probe_status='removed' THEN 'removed' WHEN c.checked_at < now()-interval '3 minutes' THEN 'stale' ELSE c.probe_status END,
 COALESCE(r.requested>r.completed,false) FROM feature_clients c LEFT JOIN feature_probe_requests r USING(project_id,node)
 WHERE c.project_id=$1 ORDER BY c.node,c.backend,c.model`, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeatureClient{}
	for rows.Next() {
		var c FeatureClient
		if err = rows.Scan(&c.ID, &c.Instance, &c.Backend, &c.Model, &c.Node, &c.CheckedAt, &c.Status, &c.Pending); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) RequestFeatureProbe(ctx context.Context, project int64, id uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO feature_probe_requests(project_id,node,requested)
 SELECT project_id,node,1 FROM feature_clients WHERE project_id=$1 AND id=$2
 ON CONFLICT(project_id,node) DO UPDATE SET requested=feature_probe_requests.completed+1`, project, id)
	return chatUpdated(res, err)
}
func (s *Store) PendingFeatureProbe(ctx context.Context, project int64, node string) (int64, error) {
	var gen int64
	err := s.db.QueryRowContext(ctx, `SELECT coalesce((SELECT requested FROM feature_probe_requests WHERE project_id=$1 AND node=$2 AND requested>completed),0)`, project, node).Scan(&gen)
	return gen, err
}
