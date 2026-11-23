package store

import (
	"context"
	"github.com/google/uuid"
)

type FeatureClient struct {
	ID       uuid.UUID `json:"id"`
	Instance string    `json:"instance"`
	Backend  string    `json:"backend"`
	Model    string    `json:"model"`
}

func (s *Store) ReportFeatureClients(ctx context.Context, project int64, node string, clients []FeatureClient) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE feature_clients SET seen_at='-infinity' WHERE project_id=$1 AND node=$2", project, node); err != nil {
		return err
	}
	for _, c := range clients {
		if _, err = tx.ExecContext(ctx, `INSERT INTO feature_clients(project_id,node,instance,backend,model) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(project_id,node,instance,backend,model) DO UPDATE SET seen_at=now()`, project, node, c.Instance, c.Backend, c.Model); err != nil {
			return err
		}
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
