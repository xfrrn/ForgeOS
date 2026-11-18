package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

type MaterialInput struct {
	PreviousRevision int    `json:"previous_revision"`
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	Source           string `json:"source"`
	Content          string `json:"content"`
}

var materialKey = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._-]{0,95}$`)

func ValidMaterialKey(key string) bool { return materialKey.MatchString(key) }
func ValidMaterialKind(kind string) bool {
	return kind == "material" || kind == "profile" || kind == "environment" || kind == "memory"
}
func (m MaterialInput) Valid() bool {
	return m.PreviousRevision >= 0 && m.PreviousRevision < 2147483647 && ValidMaterialKind(m.Kind) &&
		len(m.Title) <= 256 && strings.TrimSpace(m.Title) != "" && len(m.Source) <= 2048 && strings.TrimSpace(m.Source) != "" &&
		len(m.Content) <= 256<<10 && strings.TrimSpace(m.Content) != "" &&
		!strings.ContainsRune(m.Title, 0) && !strings.ContainsRune(m.Source, 0) && !strings.ContainsRune(m.Content, 0)
}

func (s *Store) SaveMaterial(ctx context.Context, project int64, key string, m MaterialInput) (json.RawMessage, error) {
	if !ValidMaterialKey(key) || !m.Valid() {
		return nil, ErrConflict
	}
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	var doc json.RawMessage
	// The preceding immutable revision must exist. Concurrent retries meet at the unique key.
	err := s.db.QueryRowContext(ctx, `INSERT INTO project_materials AS m
		(project_id,key,revision,kind,title,source,content,content_sha256)
		SELECT $1,$2,$3::int+1,$4,$5,$6,$7,encode(sha256(convert_to($7,'UTF8')),'hex')
		WHERE $3=0 OR EXISTS(SELECT 1 FROM project_materials WHERE project_id=$1 AND key=$2 AND revision=$3)
		ON CONFLICT (project_id,key,revision) DO UPDATE SET title=EXCLUDED.title
		WHERE (m.kind,m.title,m.source,m.content)=(EXCLUDED.kind,EXCLUDED.title,EXCLUDED.source,EXCLUDED.content)
		RETURNING to_jsonb(m)`, project, key, m.PreviousRevision, m.Kind, m.Title, m.Source, m.Content).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	return doc, err
}

func (s *Store) Material(ctx context.Context, project int64, key string, revision int) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(m) FROM project_materials m WHERE project_id=$1 AND key=$2
		AND ($3::int=0 OR revision=$3) ORDER BY revision DESC LIMIT 1`, project, key, revision).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) SearchMaterials(ctx context.Context, project int64, kind, query, after string) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	if query != "" && after == "" && s.embeddings != nil {
		return s.hybridMaterials(ctx, project, kind, query)
	}
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `WITH latest AS (
		SELECT DISTINCT ON (key) * FROM project_materials WHERE project_id=$1 AND key>$4 ORDER BY key,revision DESC
	), matches AS (
		SELECT m.*,to_tsvector('simple',title || ' ' || content) AS words,plainto_tsquery('simple',$3) AS terms
		FROM latest m WHERE ($2='' OR kind=$2)
	), page AS (
		SELECT project_id,key,revision,kind,title,source,imported_at,content_sha256,'current'::text AS status,
		       start_char,least(length(content),start_char+width) AS end_char,
		       substring(content FROM start_char+1 FOR width) AS excerpt,
		       ts_rank(words,terms) AS rank
		FROM matches CROSS JOIN LATERAL (
			SELECT CASE WHEN $3='' THEN 0 ELSE greatest(0,regexp_instr(content,$3,1,1,0,'iq')-65) END AS start_char,
			       greatest(500,length($3)+64) AS width
		) snippet
		WHERE $3='' OR words @@ terms OR strpos(lower(title || ' ' || content),lower($3))>0
		ORDER BY rank DESC,key LIMIT 30
	) SELECT COALESCE(jsonb_agg(to_jsonb(p)-'rank' ORDER BY rank DESC,key),'[]') FROM page p`, project, kind, query, after).Scan(&doc)
	return doc, err
}
