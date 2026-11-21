package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"forgeos/internal/localrepo"
)

func (s *Store) indexDocumentBlob(ctx context.Context, project int64, repo, blob string, dimensions int) error {
	var cached bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM document_embeddings
		WHERE project_id=$1 AND blob=$2 AND model=$3)`, project, blob, s.embeddings.model).Scan(&cached); err != nil || cached {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	source, err := localrepo.ReadDocumentParts(ctx, repo, blob, func(_ int, offset int64, part string) error {
		runes := []rune(part)
		offsets := make([]int, len(runes)+1)
		for i, r := range runes {
			offsets[i+1] = offsets[i] + utf8.RuneLen(r)
		}
		passages := materialPassages(runes)
		for i := 0; i < len(passages); i += 16 {
			batch := passages[i:min(i+16, len(passages))]
			type record struct {
				Start     int64     `json:"start_byte"`
				End       int64     `json:"end_byte"`
				Text      string    `json:"content"`
				Embedding []float32 `json:"embedding"`
			}
			records := make([]record, len(batch))
			inputs := make([]string, len(batch))
			for j, p := range batch {
				a, b := offsets[p.Start], offsets[p.End]
				inputs[j] = part[a:b]
				records[j] = record{Start: offset + int64(a), End: offset + int64(b), Text: inputs[j]}
			}
			vectors, err := s.embeddings.encode(ctx, inputs)
			if err != nil {
				return err
			}
			for j, v := range vectors {
				if len(v) != dimensions {
					return errors.New("document embedding dimensions differ from query")
				}
				records[j].Embedding = v
			}
			raw, err := json.Marshal(records)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO document_embeddings(project_id,blob,model,start_byte,end_byte,words,embedding)
				SELECT $1,$2,$3,start_byte,end_byte,to_tsvector('simple',content),embedding FROM jsonb_to_recordset($4)
				AS p(start_byte bigint,end_byte bigint,content text,embedding real[]) ORDER BY start_byte ON CONFLICT DO NOTHING`,
				project, blob, s.embeddings.model, string(raw))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if source.Problem != "" {
		return errors.New("committed text unavailable")
	}
	return tx.Commit()
}

func (s *Store) SearchDocuments(ctx context.Context, project int64, repo, revision, query string) (localrepo.DocumentSearch, error) {
	commit, err := localrepo.DocumentRevision(ctx, repo, revision)
	if err != nil {
		return localrepo.DocumentSearch{}, err
	}
	return s.SearchDocumentCommit(ctx, project, repo, commit, query)
}

func (s *Store) SearchDocumentCommit(ctx context.Context, project int64, repo, commit, query string) (localrepo.DocumentSearch, error) {
	if s.embeddings == nil {
		return localrepo.SearchDocumentCommit(ctx, repo, commit, query, nil)
	}
	type file struct {
		Path    string `json:"path"`
		Blob    string `json:"blob"`
		Literal bool   `json:"literal"`
	}
	var vector []float32
	var files []file
	literals := map[string]localrepo.DocumentMatch{}
	result, err := localrepo.SearchDocumentCommit(ctx, repo, commit, query, func(hit localrepo.DocumentMatch) error {
		files = append(files, file{hit.Path, hit.Blob, hit.Excerpt != ""})
		if hit.Excerpt != "" {
			literals[hit.Path] = hit
		}
		if vector == nil {
			v, err := s.embeddings.textVector(ctx, query)
			if err != nil {
				return err
			}
			vector = v
		}
		if err := s.indexDocumentBlob(ctx, project, repo, hit.Blob, len(vector)); err != nil {
			return fmt.Errorf("index document blob %s: %w", hit.Blob, err)
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		return result, err
	}
	v, err := json.Marshal(vector)
	if err != nil {
		return result, err
	}
	scope, err := json.Marshal(files)
	if err != nil {
		return result, err
	}
	rows, err := s.db.QueryContext(ctx, `WITH files AS (
		SELECT * FROM jsonb_to_recordset($1) AS f(path text,blob text,literal boolean)
	), passages AS MATERIALIZED (
		SELECT f.*,e.start_byte,e.end_byte,e.embedding::vector <=> $4::vector AS distance,
			ts_rank(e.words,plainto_tsquery('simple',$5)) AS rank,e.words @@ plainto_tsquery('simple',$5) AS keyword
		FROM files f JOIN document_embeddings e ON e.blob=f.blob AND e.project_id=$2 AND e.model=$3
	), words AS (
		SELECT DISTINCT ON (path) path,start_byte,end_byte,rank,literal FROM passages WHERE literal OR keyword
		ORDER BY path,rank DESC,start_byte
	), lexical AS (
		SELECT *,row_number() OVER (ORDER BY literal DESC,rank DESC,path) AS pos FROM words
	), nearest AS (
		SELECT DISTINCT ON (path) path,start_byte,end_byte,distance FROM passages WHERE distance<1
		ORDER BY path,distance,start_byte
	), semantic AS (
		SELECT *,row_number() OVER (ORDER BY distance,path) AS pos FROM nearest
	) SELECT f.path,f.blob,coalesce(l.start_byte,s.start_byte),coalesce(l.end_byte,s.end_byte)
		FROM files f LEFT JOIN lexical l USING(path) LEFT JOIN semantic s USING(path)
		WHERE l.pos IS NOT NULL OR s.pos IS NOT NULL
		ORDER BY coalesce(1.0/(60+l.pos),0)+coalesce(1.0/(60+s.pos),0) DESC,f.path LIMIT 31`,
		string(scope), project, s.embeddings.model, string(v), query)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var h localrepo.DocumentMatch
		if err := rows.Scan(&h.Path, &h.Blob, &h.Start, &h.End); err != nil {
			return result, err
		}
		if len(result.Items) == 30 {
			result.More = true
			break
		}
		if literal, ok := literals[h.Path]; ok {
			h = literal
		} else {
			h.ExcerptStart = h.Start
			_, err := localrepo.ReadDocumentParts(ctx, repo, h.Blob, func(_ int, offset int64, part string) error {
				if h.Start >= offset && h.End <= offset+int64(len(part)) {
					h.Excerpt = part[h.Start-offset : h.End-offset]
				}
				return nil
			})
			if err != nil {
				return result, fmt.Errorf("read matched document blob %s: %w", h.Blob, err)
			}
			if h.Excerpt == "" {
				return result, errors.New("indexed passage unavailable in committed blob")
			}
		}
		result.Items = append(result.Items, h)
	}
	return result, rows.Err()
}
