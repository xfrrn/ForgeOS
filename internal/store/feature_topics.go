package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"time"
)

type FeatureTopic struct {
	ID      int64     `json:"id"`
	Title   string    `json:"title"`
	Profile uuid.UUID `json:"profile_id"`
	Updated time.Time `json:"updated_at"`
}

const featureFields = `SELECT f.id,f.title,coalesce(last_turn.model_profile_id,f.initial_profile),coalesce(last_turn.created_at,f.created_at)
 FROM feature_topics f LEFT JOIN LATERAL (SELECT model_profile_id,created_at FROM project_chat_turns t
 WHERE t.project_id=f.project_id AND t.feature_id=f.id ORDER BY t.id DESC LIMIT 1) last_turn ON true WHERE f.project_id=$1`

func (s *Store) Feature(ctx context.Context, project, id int64) (FeatureTopic, error) {
	var f FeatureTopic
	err := s.db.QueryRowContext(ctx, featureFields+" AND f.id=$2", project, id).Scan(&f.ID, &f.Title, &f.Profile, &f.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return f, err
}
func (s *Store) Features(ctx context.Context, project int64) ([]FeatureTopic, error) {
	rows, err := s.db.QueryContext(ctx, featureFields+" ORDER BY 4 DESC,f.id DESC", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeatureTopic{}
	for rows.Next() {
		var f FeatureTopic
		if err := rows.Scan(&f.ID, &f.Title, &f.Profile, &f.Updated); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Store) CreateFeature(ctx context.Context, project int64, client, profile uuid.UUID, title string) (FeatureTopic, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO feature_topics(project_id,client_id,initial_profile,title)
 SELECT p.id,$2,$3,$4 FROM projects p WHERE p.id=$1 AND (EXISTS(SELECT 1 FROM chat_models WHERE id=$3 AND revision IS NOT NULL) OR EXISTS(SELECT 1 FROM feature_clients WHERE project_id=p.id AND id=$3 AND seen_at>now()-interval '3 minutes'))
 ON CONFLICT(project_id,client_id) DO NOTHING RETURNING id`, project, client, profile, title).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, "SELECT id FROM feature_topics WHERE project_id=$1 AND client_id=$2 AND initial_profile=$3 AND title=$4", project, client, profile, title).Scan(&id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return FeatureTopic{}, ErrConflict
	}
	if err != nil {
		return FeatureTopic{}, err
	}
	return s.Feature(ctx, project, id)
}
