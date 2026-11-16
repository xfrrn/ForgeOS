package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/lib/pq"
)

type ShareDraft struct {
	MaterialKey string `json:"material_key"`
	Revision    int    `json:"revision"`
	Recipient   int64  `json:"recipient"`
	Title       string `json:"title"`
	Content     string `json:"content"`
}

func (d ShareDraft) Valid(project int64) bool {
	return ValidMaterialKey(d.MaterialKey) && d.Revision > 0 && d.Recipient > 0 && d.Recipient != project &&
		strings.TrimSpace(d.Title) != "" && len(d.Title) <= 256 && !strings.ContainsRune(d.Title, 0) &&
		strings.TrimSpace(d.Content) != "" && len(d.Content) <= 8192 && !strings.ContainsRune(d.Content, 0)
}

type ShareReviewInput struct {
	ID int64 `json:"id"`
	ShareDraft
	OriginalTitle   string `json:"original_title"`
	OriginalSource  string `json:"original_source"`
	OriginalContent string `json:"original_content"`
}

func (s *Store) DraftShare(ctx context.Context, project int64, d ShareDraft) (json.RawMessage, error) {
	if !d.Valid(project) {
		return nil, ErrConflict
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO knowledge_shares AS s(project_id,material_key,revision,recipient,title,content,digest)
		SELECT project_id,key,revision,$4,$5,$6,encode(sha256(convert_to(jsonb_build_array($5::text,$6::text)::text,'UTF8')),'hex')
		FROM project_materials WHERE project_id=$1 AND key=$2 AND revision=$3
		AND strpos(lower($5 || ' ' || $6),lower(trim(content)))=0
		ON CONFLICT(project_id,material_key,revision,recipient,digest) DO UPDATE SET digest=EXCLUDED.digest
		RETURNING id`, project, d.MaterialKey, d.Revision, d.Recipient, d.Title, d.Content).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	var constraint *pq.Error
	if errors.As(err, &constraint) && constraint.Code == "23503" {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.ShareReceipt(ctx, project, id)
}

func (s *Store) ShareReceipt(ctx context.Context, project, id int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(s) FROM knowledge_share_receipts s WHERE project_id=$1 AND id=$2`, project, id).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) OutgoingShares(ctx context.Context, project, before int64) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	var page json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY id DESC),'[]') FROM
		(SELECT * FROM knowledge_share_receipts WHERE project_id=$1 AND ($2::bigint=0 OR id<$2)
		 ORDER BY id DESC LIMIT 30) r`, project, before).Scan(&page)
	return page, err
}

func (s *Store) ClaimShareReview(ctx context.Context, project int64) (*ManagementJob, error) {
	job := ManagementJob{ProjectID: project, MinimumLevel: "D3", Share: &ShareReviewInput{}}
	d := job.Share
	err := s.db.QueryRowContext(ctx, `WITH candidate AS (
		SELECT id FROM knowledge_shares WHERE project_id=$1 AND review IS NULL AND revoked_at IS NULL
		AND (review_until IS NULL OR review_until<=now()) ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED)
		UPDATE knowledge_shares s SET review_lease=gen_random_uuid(),review_until=now()+interval '6 minutes'
		FROM candidate c, project_materials m WHERE s.id=c.id AND (m.project_id,m.key,m.revision)=(s.project_id,s.material_key,s.revision)
		RETURNING s.id,s.material_key,s.revision,s.recipient,s.title,s.content,m.title,m.source,m.content,s.review_lease`, project).
		Scan(&d.ID, &d.MaterialKey, &d.Revision, &d.Recipient, &d.Title, &d.Content, &d.OriginalTitle, &d.OriginalSource, &d.OriginalContent, &job.Lease)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &job, err
}

func (s *Store) ReviewShare(ctx context.Context, project, id int64, lease string, review ManagementReview) error {
	if review.ValidationMinimum != "" {
		return ErrConflict
	}
	if err := review.validate(); err != nil {
		return err
	}
	review.EffectiveLevel = "D3"
	doc, err := json.Marshal(review)
	if err != nil {
		return err
	}
	var found int64
	err = s.db.QueryRowContext(ctx, `UPDATE knowledge_shares SET review=$4::jsonb,reviewed_at=COALESCE(reviewed_at,now())
		WHERE project_id=$1 AND id=$2 AND review_lease::text=$3
		AND (review=$4::jsonb OR (review IS NULL AND review_until>now() AND revoked_at IS NULL))
		RETURNING id`, project, id, lease, string(doc)).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}

func (s *Store) SetSharePublished(ctx context.Context, project, id int64, digest string, publish bool) (json.RawMessage, error) {
	query := `UPDATE knowledge_shares SET revoked_at=COALESCE(revoked_at,now()) WHERE project_id=$1 AND id=$2 AND digest=$3 RETURNING id`
	if publish {
		query = `UPDATE knowledge_shares SET published_at=COALESCE(published_at,now()) WHERE project_id=$1 AND id=$2
			AND digest=$3 AND review->>'verdict'='approve' AND revoked_at IS NULL RETURNING id`
	}
	var found int64
	err := s.db.QueryRowContext(ctx, query, project, id, digest).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return s.ShareReceipt(ctx, project, id)
}

func (s *Store) SharedExperience(ctx context.Context, recipient, before int64) (json.RawMessage, error) {
	if _, err := s.Project(ctx, recipient); err != nil {
		return nil, err
	}
	var doc json.RawMessage
	// Only the reviewed copy is visible. Its private source and reviewer notes stay in the origin project.
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY id DESC),'[]') FROM (
		SELECT id,title,content,digest,published_at FROM knowledge_shares WHERE recipient=$1 AND published_at IS NOT NULL
		AND revoked_at IS NULL AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT 30) s`, recipient, before).Scan(&doc)
	return doc, err
}
