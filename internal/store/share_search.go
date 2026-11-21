package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/lib/pq"
)

// Select and embed only the approved copy released to this recipient.
func (s *Store) indexShares(ctx context.Context, project int64, dimensions int) error {
	var after int64
	for {
		var id int64
		var body string
		err := s.db.QueryRowContext(ctx, `SELECT id,title||E'\n'||content FROM visible_knowledge_shares k WHERE recipient=$1
            AND id>$2 AND NOT EXISTS(SELECT 1 FROM share_embeddings e WHERE e.share_id=k.id AND e.model=$3)
            ORDER BY id LIMIT 1`, project, after, s.embeddings.model).Scan(&id, &body)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		v, err := s.embeddings.textVector(ctx, body)
		if err != nil {
			return err
		}
		if len(v) != dimensions {
			return errors.New("share embedding dimensions differ from query")
		}
		// Check the pointer again after encoding. Lock only the series, in the
		// same order as publication and withdrawal, until the cache is written.
		_, err = s.db.ExecContext(ctx, `WITH current_copy AS (SELECT current_share_id FROM knowledge_share_series
			WHERE recipient=$1 AND current_share_id=$2 FOR SHARE)
			INSERT INTO share_embeddings(share_id,model,embedding) SELECT current_share_id,$3,$4 FROM current_copy ON CONFLICT DO NOTHING`,
			project, id, s.embeddings.model, pq.Array(v))
		if err != nil {
			return err
		}
		after = id
	}
}

func (s *Store) SearchSharedExperience(ctx context.Context, project int64, query string) (json.RawMessage, error) {
	return s.SearchReceivedShares(ctx, project, query, false)
}

func (s *Store) SearchReceivedShares(ctx context.Context, project int64, query string, versions bool) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	args := []any{project, query}
	semantic := `SELECT id,0::bigint AS pos FROM scoped WHERE false`
	if s.embeddings != nil {
		v, err := s.embeddings.textVector(ctx, query)
		if err != nil {
			return nil, err
		}
		if err := s.indexShares(ctx, project, len(v)); err != nil {
			return nil, err
		}
		vector, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		args = append(args, s.embeddings.model, string(vector))
		semantic = `WITH eligible AS MATERIALIZED (SELECT s.id,e.embedding FROM scoped s
            JOIN share_embeddings e ON e.share_id=s.id AND e.model=$3)
            SELECT id,row_number() OVER (ORDER BY distance,id DESC) AS pos FROM
            (SELECT id,embedding::vector <=> $4::vector AS distance FROM eligible) hits WHERE distance<1`
	}
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `WITH scoped AS MATERIALIZED (
        SELECT `+sharedCopyFields(versions)+` FROM visible_knowledge_shares WHERE recipient=$1
    ), words AS (
        SELECT id,title||' '||content AS body FROM scoped
    ), lexical AS (
        SELECT id,row_number() OVER (ORDER BY ts_rank(to_tsvector('simple',body),plainto_tsquery('simple',$2)) DESC,id DESC) AS pos
        FROM words WHERE to_tsvector('simple',body) @@ plainto_tsquery('simple',$2) OR strpos(lower(body),lower($2))>0
    ), semantic AS (`+semantic+`), page AS (
        SELECT s.*,coalesce(1.0/(60+l.pos),0)+coalesce(1.0/(60+v.pos),0) AS score FROM scoped s
        LEFT JOIN lexical l USING(id) LEFT JOIN semantic v USING(id)
        WHERE l.id IS NOT NULL OR v.id IS NOT NULL ORDER BY score DESC,id DESC LIMIT 30
    ) SELECT coalesce(jsonb_agg(to_jsonb(p)-'score' ORDER BY score DESC,id DESC),'[]') FROM page p`, args...).Scan(&doc)
	return doc, err
}
