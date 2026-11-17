package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type KnowledgeFact struct {
	Statement string `json:"statement"`
	Quote     string `json:"quote"`
}

type Extraction struct {
	Session string          `json:"session"`
	Facts   []KnowledgeFact `json:"facts"`
}

type CurationJob struct {
	ID         int64           `json:"id"`
	Lease      string          `json:"lease"`
	Material   json.RawMessage `json:"material"`
	Extraction *Extraction     `json:"extraction,omitempty"`
}

func (s *Store) ClaimCuration(ctx context.Context, project int64, review bool) (*CurationJob, error) {
	pending, lease, until := "extraction IS NULL", "extraction_lease", "extraction_until"
	if review {
		pending, lease, until = "extraction IS NOT NULL AND jsonb_array_length(extraction->'facts')>0 AND review IS NULL", "review_lease", "review_until"
	}
	var job CurationJob
	var extraction []byte
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
		SELECT id FROM material_curations WHERE project_id=$1 AND `+pending+` AND (`+until+` IS NULL OR `+until+`<=now())
		ORDER BY `+until+` NULLS FIRST,id LIMIT 1 FOR UPDATE SKIP LOCKED)
		UPDATE material_curations c SET `+lease+`=gen_random_uuid(),`+until+`=now()+interval '6 minutes'
		FROM candidate,project_materials m WHERE c.id=candidate.id AND (m.project_id,m.key,m.revision)=(c.project_id,c.key,c.revision)
		RETURNING c.id,c.`+lease+`,to_jsonb(m),c.extraction`, project).Scan(&job.ID, &job.Lease, &job.Material, &extraction)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if extraction != nil {
		if err = json.Unmarshal(extraction, &job.Extraction); err != nil {
			return nil, err
		}
	}
	return &job, nil
}

func (s *Store) RecordExtraction(ctx context.Context, project, id int64, lease string, out Extraction) error {
	if out.Session == "" || len(out.Session) > 128 || strings.Trim(out.Session, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" ||
		out.Facts == nil || len(out.Facts) > 8 {
		return ErrConflict
	}
	for _, f := range out.Facts {
		if strings.TrimSpace(f.Statement) == "" || len(f.Statement) > 2048 || strings.TrimSpace(f.Quote) == "" || len(f.Quote) > 2048 ||
			strings.ContainsRune(f.Statement, 0) || strings.ContainsRune(f.Quote, 0) {
			return ErrConflict
		}
	}
	doc, err := json.Marshal(out)
	if err != nil {
		return err
	}
	var recorded int64
	err = s.db.QueryRowContext(ctx, `UPDATE material_curations c SET extraction=$4::jsonb,extracted_at=COALESCE(extracted_at,now())
		FROM project_materials m WHERE c.project_id=$1 AND c.id=$2 AND c.extraction_lease::text=$3
		AND (c.extraction=$4::jsonb OR (c.extraction IS NULL AND c.extraction_until>now()))
		AND (c.project_id,c.key,c.revision)=(m.project_id,m.key,m.revision)
		AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements($4::jsonb->'facts') f WHERE strpos(m.content,f->>'quote')=0)
		RETURNING c.id`, project, id, lease, string(doc)).Scan(&recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}

func (s *Store) ReviewCuration(ctx context.Context, project, id int64, lease string, review ManagementReview) error {
	if review.validate() != nil || review.ValidationMinimum != "" || strings.ContainsRune(review.Rationale, 0) || strings.ContainsRune(review.Evidence, 0) {
		return ErrConflict
	}
	doc, err := json.Marshal(review)
	if err != nil {
		return err
	}
	var recorded int64
	err = s.db.QueryRowContext(ctx, `UPDATE material_curations SET review=$4::jsonb,reviewed_at=COALESCE(reviewed_at,now())
		WHERE project_id=$1 AND id=$2 AND review_lease::text=$3 AND extraction->>'session'<>$5
		AND (review=$4::jsonb OR (review IS NULL AND review_until>now())) RETURNING id`, project, id, lease, string(doc), review.SessionID).Scan(&recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}

func (s *Store) MaterialCuration(ctx context.Context, project int64, key string, revision int) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT jsonb_build_object('id',c.id,'key',c.key,'revision',c.revision,'content_sha256',m.content_sha256,
		'extraction',c.extraction,'extracted_at',c.extracted_at,'review',c.review,'reviewed_at',c.reviewed_at)
		FROM material_curations c JOIN project_materials m USING(project_id,key,revision)
		WHERE project_id=$1 AND key=$2 AND revision=$3`, project, key, revision).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) ProjectExperience(ctx context.Context, project, before int64) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(a) ORDER BY id DESC),'[]') FROM (
		SELECT c.id,c.key,c.revision,m.title,m.source,m.content_sha256,c.extraction->'facts' AS facts,c.reviewed_at
		FROM material_curations c JOIN project_materials m USING(project_id,key,revision)
		WHERE c.project_id=$1 AND ($2::bigint=0 OR c.id<$2) AND c.review->>'verdict'='approve' AND c.review->>'level'='D1'
		AND NOT EXISTS (SELECT 1 FROM project_materials n WHERE n.project_id=c.project_id AND n.key=c.key AND n.revision>c.revision)
		ORDER BY c.id DESC LIMIT 30) a`, project, before).Scan(&doc)
	return doc, err
}
