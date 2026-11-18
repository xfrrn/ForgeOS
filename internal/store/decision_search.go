package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/lib/pq"
)

const searchableDecisions = `WITH scoped AS MATERIALIZED (
	SELECT d.* FROM project_decision_index d WHERE project_id=$1 AND ($2='' OR status=$2)
)`

// Decision text is immutable. Recheck the effective status before caching or ranking it.
func (s *Store) indexDecisions(ctx context.Context, project int64, status string, dimensions int) error {
	var after int64
	for {
		var id int64
		var body string
		err := s.db.QueryRowContext(ctx, `
			SELECT id,statement||E'\n'||validity FROM project_decision_index d
			WHERE project_id=$1 AND ($2='' OR status=$2) AND id>$3 AND NOT EXISTS (
				SELECT 1 FROM decision_embeddings e WHERE e.decision_id=d.id AND e.model=$4)
			ORDER BY id LIMIT 1`, project, status, after, s.embeddings.model).Scan(&id, &body)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		vector, err := s.embeddings.textVector(ctx, body)
		if err != nil {
			return err
		}
		if len(vector) != dimensions {
			return errors.New("decision embedding dimensions differ from query")
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO decision_embeddings(decision_id,model,embedding)
			SELECT id,$4,$5 FROM project_decision_index WHERE project_id=$1 AND ($2='' OR status=$2) AND id=$3
			ON CONFLICT DO NOTHING`, project, status, id, s.embeddings.model, pq.Array(vector))
		if err != nil {
			return err
		}
		after = id
	}
}

func (s *Store) SearchDecisions(ctx context.Context, project int64, status, query string) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	args := []any{project, status, query}
	semantic := `SELECT id,0::bigint AS position FROM scoped WHERE false`
	if s.embeddings != nil {
		v, err := s.embeddings.textVector(ctx, query)
		if err != nil {
			return nil, err
		}
		if err := s.indexDecisions(ctx, project, status, len(v)); err != nil {
			return nil, err
		}
		vector, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		args = append(args, s.embeddings.model, string(vector))
		semantic = `WITH eligible AS MATERIALIZED (
			SELECT d.id,e.embedding FROM scoped d JOIN decision_embeddings e ON e.decision_id=d.id AND e.model=$4)
			SELECT id,row_number() OVER (ORDER BY distance,id DESC) AS position FROM (
				SELECT id,embedding::vector <=> $5::vector AS distance FROM eligible) matches WHERE distance<1`
	}
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, searchableDecisions+`, words AS (
		SELECT id,statement||' '||validity||' '||source AS body FROM scoped
	), lexical AS (
		SELECT id,row_number() OVER (ORDER BY ts_rank(to_tsvector('simple',body),plainto_tsquery('simple',$3)) DESC,id DESC) AS position
		FROM words WHERE to_tsvector('simple',body) @@ plainto_tsquery('simple',$3) OR strpos(lower(body),lower($3))>0
	), semantic AS (`+semantic+`), page AS (
		SELECT d.*,coalesce(1.0/(60+l.position),0)+coalesce(1.0/(60+v.position),0) AS score
		FROM scoped d LEFT JOIN lexical l USING(id) LEFT JOIN semantic v USING(id)
		WHERE l.id IS NOT NULL OR v.id IS NOT NULL ORDER BY score DESC,id DESC LIMIT 30
	) SELECT coalesce(jsonb_agg(to_jsonb(p)-'score' ORDER BY score DESC,id DESC),'[]') FROM page p`, args...).Scan(&result)
	return result, err
}
