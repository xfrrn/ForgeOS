package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
)

type ShareDraft struct {
	MaterialKey        string `json:"material_key,omitempty"`
	Revision           int    `json:"revision,omitempty"`
	ExperienceID       int64  `json:"experience_id,omitempty"`
	ExperienceRevision int    `json:"experience_revision,omitempty"`
	Recipient          int64  `json:"recipient"`
	Title              string `json:"title"`
	Content            string `json:"content"`
}

func (d ShareDraft) Valid(project int64) bool {
	material := ValidMaterialKey(d.MaterialKey) && d.Revision > 0 && d.Revision <= math.MaxInt32 && d.ExperienceID == 0 && d.ExperienceRevision == 0
	experience := d.MaterialKey == "" && d.Revision == 0 && d.ExperienceID > 0 && d.ExperienceRevision > 0 && d.ExperienceRevision <= math.MaxInt32
	return (material || experience) && d.Recipient > 0 && d.Recipient != project && validShareCopy(d.Title, d.Content)
}

func validShareCopy(title, content string) bool {
	return strings.TrimSpace(title) != "" && len(title) <= 256 && !strings.ContainsRune(title, 0) &&
		strings.TrimSpace(content) != "" && len(content) <= 8192 && !strings.ContainsRune(content, 0)
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
	return s.createShareVersion(ctx, project, 0, d, ShareVersionDraft{})
}

const shareReceiptJSON = `to_jsonb(s) || jsonb_build_object('review_processes',COALESCE((SELECT jsonb_agg(p.process || jsonb_strip_nulls(jsonb_build_object('node',p.node,'attempt',p.id,'observed_at',p.observed_at,'resumed_from',NULLIF(p.resumed_from,0),'missing_session',NULLIF(p.missing_session,false))) ORDER BY p.id) FROM management_processes p WHERE p.share_id=s.id),'[]'::jsonb))`

func (s *Store) ShareReceipt(ctx context.Context, project, id int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT `+shareReceiptJSON+` FROM knowledge_share_receipts s WHERE project_id=$1 AND id=$2`, project, id).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) OutgoingShares(ctx context.Context, project, before int64) (json.RawMessage, error) {
	return s.outgoingSharePage(ctx, project, before, "", 0, false)
}

func (s *Store) OutgoingSharePage(ctx context.Context, project, before int64, query string, latest bool) (json.RawMessage, error) {
	return s.outgoingSharePage(ctx, project, before, query, 0, latest)
}

func (s *Store) ShareVersions(ctx context.Context, project, share, before int64) (json.RawMessage, error) {
	var series int64
	err := s.db.QueryRowContext(ctx, `SELECT series_id FROM knowledge_shares WHERE project_id=$1 AND id=$2`, project, share).Scan(&series)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.outgoingSharePage(ctx, project, before, "", series, false)
}

func (s *Store) outgoingSharePage(ctx context.Context, project, before int64, query string, series int64, latest bool) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	var page json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(`+shareReceiptJSON+` ORDER BY id DESC),'[]') FROM
		(SELECT k.* FROM knowledge_share_receipts k WHERE project_id=$1 AND ($2::bigint=0 OR id<$2)
		 AND ($3='' OR strpos(lower(title||' '||content),lower($3))>0) AND ($4::bigint=0 OR series_id=$4)
		 AND (NOT $5 OR NOT EXISTS (SELECT 1 FROM knowledge_shares newer WHERE newer.series_id=k.series_id AND newer.share_version>k.share_version
		     AND ($3='' OR strpos(lower(newer.title||' '||newer.content),lower($3))>0)))
		 ORDER BY id DESC LIMIT 30) s`, project, before, query, series, latest).Scan(&page)
	return page, err
}

func (s *Store) ClaimShareReview(ctx context.Context, project int64, supportsDeadline bool) (*ManagementJob, error) {
	job := ManagementJob{ProjectID: project, MinimumLevel: "D3", Share: &ShareReviewInput{}, CaptureProfile: true}
	d := job.Share
	tx, pool, err := s.beginManagementClaim(ctx, project, "share_review", supportsDeadline)
	if tx == nil {
		return nil, err
	}
	defer tx.Rollback()
	if allowed, err := knowledgeModelAllowed(ctx, tx.Tx, project, "management"); err != nil || !allowed {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT k.id FROM knowledge_shares k JOIN knowledge_share_series g ON g.id=k.series_id
		WHERE k.project_id=$1 `+knowledgeAttemptEligible(ctx, "k.id", "share_review")+` AND k.review IS NULL AND k.revoked_at IS NULL AND k.base_epoch=g.publication_epoch
		AND (k.review_until IS NULL OR k.review_until<=clock_timestamp()) ORDER BY k.id LIMIT 1 FOR UPDATE OF k SKIP LOCKED)
		UPDATE knowledge_shares s SET review_lease=gen_random_uuid(),review_until=clock_timestamp()+interval '6 minutes',model_budget=$2
		FROM candidate c, knowledge_share_sources m WHERE s.id=c.id AND m.project_id=s.project_id
		AND ((m.material_key,m.revision)=(s.material_key,s.revision) OR (m.experience_id,m.experience_revision)=(s.experience_id,s.experience_revision))
		RETURNING s.id,coalesce(s.material_key,''),coalesce(s.revision,0),coalesce(s.experience_id,0),coalesce(s.experience_revision,0),s.recipient,s.title,s.content,m.title,m.source,m.content,s.review_lease,CASE WHEN $3 THEN s.review_until END`, project, pool, supportsDeadline).
		Scan(&d.ID, &d.MaterialKey, &d.Revision, &d.ExperienceID, &d.ExperienceRevision, &d.Recipient, &d.Title, &d.Content, &d.OriginalTitle, &d.OriginalSource, &d.OriginalContent, &job.Lease, &job.LeaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, tx.Commit()
}

func (s *Store) ReviewShare(ctx context.Context, project, id int64, lease string, review ManagementReview) error {
	if review.ValidationMinimum != "" {
		return ErrConflict
	}
	if err := review.validate(); err != nil {
		return err
	}
	review.EffectiveLevel = "D3"
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = requireKnowledgeModelResult(ctx, tx, project, "management", "share_review", id, lease); err != nil {
		return err
	}
	var stored []byte
	var valid, revoked bool
	err = tx.QueryRowContext(ctx, `SELECT review,review_until>clock_timestamp(),revoked_at IS NOT NULL FROM knowledge_shares
 WHERE project_id=$1 AND id=$2 AND review_lease::text=$3 FOR UPDATE`, project, id, lease).Scan(&stored, &valid, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if stored != nil {
		var previous ManagementReview
		if err = json.Unmarshal(stored, &previous); err != nil {
			return err
		}
		if !reflect.DeepEqual(previous, review) {
			return ErrConflict
		}
		return nil
	}
	if !valid {
		return ErrConflict
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{ShareID: id, Lease: lease}, review.SessionID); err != nil {
		return err
	}
	if revoked {
		// A completed revoked review returns its account slot without publishing anything.
		if _, err = tx.ExecContext(ctx, "UPDATE knowledge_shares SET review_until=clock_timestamp() WHERE id=$1", id); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrConflict
	}
	doc, err := json.Marshal(review)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE knowledge_shares SET review=$2::jsonb,reviewed_at=now() WHERE id=$1", id, string(doc)); err != nil {
		return err
	}
	return tx.Commit()
}

func sharedCopyFields(versions bool) string {
	const fields = "id,title,content,digest,published_at"
	if versions {
		return fields + ",series_id,share_version"
	}
	return fields
}

func (s *Store) ReceivedShare(ctx context.Context, project, series int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(s) FROM (SELECT `+sharedCopyFields(true)+` FROM visible_knowledge_shares WHERE recipient=$1 AND series_id=$2) s`, project, series).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

func (s *Store) SharedExperience(ctx context.Context, recipient, before int64) (json.RawMessage, error) {
	return s.ReceivedSharePage(ctx, recipient, before, false)
}

func (s *Store) ReceivedSharePage(ctx context.Context, recipient, before int64, versions bool) (json.RawMessage, error) {
	if _, err := s.Project(ctx, recipient); err != nil {
		return nil, err
	}
	var doc json.RawMessage
	// Only the reviewed copy is visible. Its private source and reviewer notes stay in the origin project.
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY id DESC),'[]') FROM (
		SELECT `+sharedCopyFields(versions)+` FROM visible_knowledge_shares WHERE recipient=$1 AND ($2::bigint=0 OR id<$2) ORDER BY id DESC LIMIT 30) s`, recipient, before).Scan(&doc)
	return doc, err
}
