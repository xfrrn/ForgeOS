package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/lib/pq"
)

const experienceFacts = `WITH facts AS MATERIALIZED (
	SELECT e.id,e.project_id,e.key,e.decision_id,e.event_id,e.revision,e.title,e.kind,e.source,e.receipt_path,
		e.content_sha256,e.reviewed_at,'approved'::text AS status,'D1'::text AS level,
		(f.value->>'fact_number')::integer AS fact_number,f.value->>'statement' AS statement,f.value->>'quote' AS quote,
		strpos(e.content,f.value->>'quote')-1 AS start_char
	FROM current_project_experience e CROSS JOIN LATERAL jsonb_array_elements(e.facts) f
	WHERE e.project_id=$1)`

func (s *Store) indexExperience(ctx context.Context, project int64, dimensions int) error {
	type fact struct {
		id     int64
		number int
		text   string
	}
	var last fact
	for {
		rows, err := s.db.QueryContext(ctx, experienceFacts+`
			SELECT id,fact_number,statement||E'\n'||quote FROM facts f
			WHERE (id,fact_number)>($2,$3) AND NOT EXISTS(SELECT 1 FROM experience_embeddings e
				WHERE (e.curation_id,e.fact_number,e.model)=(f.id,f.fact_number,$4))
			ORDER BY id,fact_number LIMIT 16`, project, last.id, last.number, s.embeddings.model)
		if err != nil {
			return err
		}
		var page []fact
		for rows.Next() {
			var f fact
			if err := rows.Scan(&f.id, &f.number, &f.text); err != nil {
				rows.Close()
				return err
			}
			page = append(page, f)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, f := range page {
			v, err := s.embeddings.textVector(ctx, f.text)
			if err != nil {
				return err
			}
			if len(v) != dimensions {
				return errors.New("experience embedding dimensions differ from query")
			}
			_, err = s.db.ExecContext(ctx, `INSERT INTO experience_embeddings(curation_id,fact_number,model,embedding)
				SELECT curation_id,fact_number,$4,$5 FROM experience_revision_facts
				WHERE project_id=$1 AND curation_id=$2 AND fact_number=$3 AND is_effective
				ON CONFLICT DO NOTHING`, project, f.id, f.number, s.embeddings.model, pq.Array(v))
			if err != nil {
				return err
			}
			last = f
		}
		if len(page) < 16 {
			return nil
		}
	}
}

func (s *Store) SearchExperience(ctx context.Context, project int64, query string) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	args := []any{project, query}
	semantic := `SELECT id,fact_number,0::bigint AS position FROM facts WHERE false`
	if s.embeddings != nil {
		v, err := s.embeddings.textVector(ctx, query)
		if err != nil {
			return nil, err
		}
		if err = s.indexExperience(ctx, project, len(v)); err != nil {
			return nil, err
		}
		vector, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		args = append(args, s.embeddings.model, string(vector))
		semantic = `WITH eligible AS MATERIALIZED (
				SELECT f.id,f.fact_number,e.embedding FROM facts f JOIN experience_embeddings e
				ON (e.curation_id,e.fact_number,e.model)=(f.id,f.fact_number,$3))
			SELECT id,fact_number,row_number() OVER (ORDER BY distance,id DESC,fact_number) AS position
			FROM (SELECT id,fact_number,embedding::vector <=> $4::vector AS distance FROM eligible) matches
			WHERE distance<1`
	}
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, experienceFacts+`, words AS (
		SELECT id,fact_number,title||' '||statement||' '||quote AS body FROM facts
	), lexical AS (
		SELECT id,fact_number,row_number() OVER (ORDER BY ts_rank(to_tsvector('simple',body),plainto_tsquery('simple',$2)) DESC,id DESC,fact_number) AS position
		FROM words WHERE to_tsvector('simple',body) @@ plainto_tsquery('simple',$2) OR strpos(lower(body),lower($2))>0
	), semantic AS (`+semantic+`), page AS (
		SELECT f.*,f.start_char+length(f.quote) AS end_char,
			coalesce(1.0/(60+l.position),0)+coalesce(1.0/(60+v.position),0) AS score
		FROM facts f LEFT JOIN lexical l USING(id,fact_number) LEFT JOIN semantic v USING(id,fact_number)
		WHERE l.position IS NOT NULL OR v.position IS NOT NULL ORDER BY score DESC,id DESC,fact_number LIMIT 30
	) SELECT coalesce(jsonb_agg(jsonb_strip_nulls(to_jsonb(p)-'score') ORDER BY score DESC,id DESC,fact_number),'[]') FROM page p`, args...).Scan(&result)
	return result, err
}
