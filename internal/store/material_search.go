package store

import (
	"context"
	"encoding/json"
	"errors"
	"unicode"
)

type materialPassage struct {
	Start  int       `json:"start_char"`
	End    int       `json:"end_char"`
	Vector []float32 `json:"embedding"`
}

func materialPassages(text []rune) []materialPassage {
	var out []materialPassage
	for start := 0; start < len(text); {
		end := min(start+96, len(text))
		if end < len(text) {
			for i := end; i > start+48; i-- {
				if unicode.IsSpace(text[i-1]) {
					end = i
					break
				}
			}
		}
		out = append(out, materialPassage{Start: start, End: end})
		if end == len(text) {
			break
		}
		start = end - 12
		for i := start; i > end-24; i-- {
			if unicode.IsSpace(text[i-1]) {
				start = i
				break
			}
		}
	}
	return out
}

const latestMaterials = `WITH scoped AS MATERIALIZED (
	SELECT project_id,key,revision FROM (
		SELECT DISTINCT ON (key) project_id,key,revision,kind FROM project_materials WHERE project_id=$1 ORDER BY key,revision DESC
	) latest WHERE $2='' OR kind=$2)`

func (s *Store) indexMaterial(ctx context.Context, project int64, key string, revision int, dimensions int) error {
	var content string
	if err := s.db.QueryRowContext(ctx, `SELECT content FROM project_materials WHERE project_id=$1 AND key=$2 AND revision=$3`, project, key, revision).Scan(&content); err != nil {
		return err
	}
	text := []rune(content)
	passages := materialPassages(text)
	const batchSize = 16
	for i := 0; i < len(passages); i += batchSize {
		batch := passages[i:min(i+batchSize, len(passages))]
		inputs := make([]string, len(batch))
		for j, p := range batch {
			inputs[j] = string(text[p.Start:p.End])
		}
		vectors, err := s.embeddings.encode(ctx, inputs)
		if err != nil {
			return err
		}
		for j, v := range vectors {
			if len(v) != dimensions {
				return errors.New("material encoder changed dimensions within one search")
			}
			batch[j].Vector = v
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := 0; i < len(passages); i += batchSize {
		raw, err := json.Marshal(passages[i:min(i+batchSize, len(passages))])
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO material_embeddings(project_id,key,revision,model,start_char,end_char,embedding)
			SELECT $1,$2,$3,$4,p.start_char,p.end_char,p.embedding FROM jsonb_to_recordset($5)
			AS p(start_char integer,end_char integer,embedding real[]) ORDER BY p.start_char ON CONFLICT DO NOTHING`,
			project, key, revision, s.embeddings.model, string(raw))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) hybridMaterials(ctx context.Context, project int64, kind, query string) (json.RawMessage, error) {
	rows, err := s.db.QueryContext(ctx, latestMaterials+`
		SELECT m.key,m.revision FROM scoped m WHERE NOT EXISTS(
			SELECT 1 FROM material_embeddings e WHERE (e.project_id,e.key,e.revision,e.model)=(m.project_id,m.key,m.revision,$3))
		ORDER BY m.key`, project, kind, s.embeddings.model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type revision struct {
		key    string
		number int
	}
	var missing []revision
	for rows.Next() {
		var m revision
		if err := rows.Scan(&m.key, &m.number); err != nil {
			return nil, err
		}
		missing = append(missing, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	v, err := s.embeddings.queryVector(ctx, query)
	if err != nil {
		return nil, err
	}
	for _, m := range missing {
		if err := s.indexMaterial(ctx, project, m.key, m.number, len(v)); err != nil {
			return nil, err
		}
	}
	vector, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Reciprocal rank fusion combines the two orderings without treating their raw scores as comparable.
	var doc json.RawMessage
	err = s.db.QueryRowContext(ctx, latestMaterials+`, docs AS MATERIALIZED (
		SELECT m.* FROM scoped JOIN project_materials m USING(project_id,key,revision)
	), words AS (
		SELECT key,to_tsvector('simple',title||' '||content) AS terms,plainto_tsquery('simple',$3) AS q,
			strpos(lower(title||' '||content),lower($3))>0 AS literal FROM docs
	), lexical AS (
		SELECT key,row_number() OVER (ORDER BY ts_rank(terms,q) DESC,key) AS position FROM words WHERE terms @@ q OR literal
	), best AS (
		SELECT DISTINCT ON (m.key) m.key,e.start_char,e.end_char,e.embedding::vector <=> $5::vector AS distance
		FROM scoped m JOIN material_embeddings e ON (e.project_id,e.key,e.revision,e.model)=(m.project_id,m.key,m.revision,$4)
		ORDER BY m.key,distance,e.start_char
	), semantic AS (
		SELECT *,row_number() OVER (ORDER BY distance,key) AS position FROM best WHERE distance<1
	), page AS (
		SELECT m.project_id,m.key,m.revision,m.kind,m.title,m.source,m.imported_at,m.content_sha256,'current'::text AS status,
			coalesce(v.start_char,0) AS start_char,coalesce(v.end_char,least(length(m.content),500)) AS end_char,
			substring(m.content FROM coalesce(v.start_char,0)+1 FOR coalesce(v.end_char-v.start_char,500)) AS excerpt,
			coalesce(1.0/(60+l.position),0)+coalesce(1.0/(60+v.position),0) AS score
		FROM docs m LEFT JOIN lexical l USING(key) LEFT JOIN semantic v USING(key)
		WHERE l.position IS NOT NULL OR v.position IS NOT NULL ORDER BY score DESC,m.key LIMIT 30
	) SELECT coalesce(jsonb_agg(to_jsonb(p)-'score' ORDER BY score DESC,key),'[]') FROM page p`,
		project, kind, query, s.embeddings.model, string(vector)).Scan(&doc)
	return doc, err
}
