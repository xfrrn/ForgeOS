package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type KnowledgeFact struct {
	Statement string `json:"statement"`
	Quote     string `json:"quote"`
}

type Extraction struct {
	Session string          `json:"session"`
	Facts   []KnowledgeFact `json:"facts"`
	Profile *AgentProfile   `json:"profile,omitempty"`
}

type CurationJob struct {
	CaptureProcess bool            `json:"capture_process,omitempty"`
	CaptureProfile bool            `json:"capture_profile,omitempty"`
	ID             int64           `json:"id"`
	Lease          string          `json:"lease"`
	LeaseUntil     *time.Time      `json:"lease_until,omitempty"`
	Material       json.RawMessage `json:"material"`
	Extraction     *Extraction     `json:"extraction,omitempty"`
}

func (s *Store) ClaimCuration(ctx context.Context, project int64, review, supportsDeadline bool) (*CurationJob, error) {
	role := "curator"
	pending, lease, until := "extraction IS NULL", "extraction_lease", "extraction_until"
	if review {
		role = "curation_review"
		pending, lease, until = "extraction IS NOT NULL AND jsonb_array_length(extraction->'facts')>0 AND review IS NULL", "review_lease", "review_until"
	}
	tx, err := s.beginRoleClaim(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	pool, available, err := modelSlot(ctx, tx.Tx, "codex", "", role, project)
	if err != nil || !available || (pool.Valid && !supportsDeadline) {
		return nil, err
	}
	var job CurationJob
	var extraction []byte
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
		SELECT id FROM material_curations WHERE project_id=$1 AND `+pending+` AND (`+until+` IS NULL OR `+until+`<=now())
		ORDER BY `+until+` NULLS FIRST,id LIMIT 1 FOR UPDATE SKIP LOCKED)
		UPDATE material_curations c SET `+lease+`=gen_random_uuid(),`+until+`=clock_timestamp()+interval '6 minutes',model_budget=$2
		FROM candidate,curation_sources s WHERE c.id=candidate.id AND s.curation_id=c.id
		RETURNING c.id,c.`+lease+`,jsonb_strip_nulls(to_jsonb(s)-'curation_id'-'receipt_path'-'is_current'),c.extraction-'profile',
		CASE WHEN $3 THEN c.`+until+` END`, project, pool, supportsDeadline).Scan(&job.ID, &job.Lease, &job.Material, &extraction, &job.LeaseUntil)
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
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	job.CaptureProfile = true
	return &job, nil
}

func (s *Store) RecordExtraction(ctx context.Context, project, id int64, lease string, out Extraction) error {
	if out.Session == "" || len(out.Session) > 128 || strings.Trim(out.Session, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" ||
		out.Facts == nil || len(out.Facts) > 8 || !validStructuredProfile(out.Profile) {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM material_curations WHERE project_id=$1 AND id=$2 AND extraction_lease::text=$3 FOR UPDATE`, project, id, lease).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{CurationID: id, CurationRole: "curator", Lease: lease}, out.Session); err != nil {
		return err
	}
	var recorded int64
	err = tx.QueryRowContext(ctx, `UPDATE material_curations c SET extraction=$4::jsonb,extracted_at=COALESCE(extracted_at,now())
		FROM curation_sources s WHERE c.project_id=$1 AND c.id=$2 AND c.extraction_lease::text=$3
		AND (c.extraction=$4::jsonb OR (c.extraction IS NULL AND c.extraction_until>now()))
		AND s.curation_id=c.id
		AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements($4::jsonb->'facts') f WHERE strpos(s.content,f->>'quote')=0)
		RETURNING c.id`, project, id, lease, string(doc)).Scan(&recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReviewCuration(ctx context.Context, project, id int64, lease string, review ManagementReview) error {
	if review.validate() != nil || review.ValidationMinimum != "" || strings.ContainsRune(review.Rationale, 0) || strings.ContainsRune(review.Evidence, 0) {
		return ErrConflict
	}
	doc, err := json.Marshal(review)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var locked int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM material_curations WHERE project_id=$1 AND id=$2 AND review_lease::text=$3 FOR UPDATE`, project, id, lease).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{CurationID: id, CurationRole: "management", Lease: lease}, review.SessionID); err != nil {
		return err
	}
	var recorded int64
	err = tx.QueryRowContext(ctx, `UPDATE material_curations SET review=$4::jsonb,reviewed_at=COALESCE(reviewed_at,now())
		WHERE project_id=$1 AND id=$2 AND review_lease::text=$3 AND extraction->>'session'<>$5
		AND (review=$4::jsonb OR (review IS NULL AND review_until>now())) RETURNING id`, project, id, lease, string(doc), review.SessionID).Scan(&recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

const curationReceipt = `SELECT jsonb_build_object('id',c.id,'revision',s.revision,'content_sha256',s.content_sha256,
		'extraction',c.extraction,'extracted_at',c.extracted_at,'review',c.review,'reviewed_at',c.reviewed_at,
        'extraction_processes',COALESCE((SELECT jsonb_agg(p.process || jsonb_strip_nulls(jsonb_build_object('node',p.node,'attempt',p.id,'observed_at',p.observed_at,'resumed_from',NULLIF(p.resumed_from,0),'missing_session',NULLIF(p.missing_session,false))) ORDER BY p.id) FROM management_processes p WHERE p.curation_id=c.id AND p.curation_role='curator'),'[]'::jsonb),
        'review_processes',COALESCE((SELECT jsonb_agg(p.process || jsonb_strip_nulls(jsonb_build_object('node',p.node,'attempt',p.id,'observed_at',p.observed_at,'resumed_from',NULLIF(p.resumed_from,0),'missing_session',NULLIF(p.missing_session,false))) ORDER BY p.id) FROM management_processes p WHERE p.curation_id=c.id AND p.curation_role='management'),'[]'::jsonb),
		'decisions',COALESCE((SELECT jsonb_agg(jsonb_build_object('fact',d.curation_fact,'id',d.id,'status',i.status) ORDER BY d.curation_fact,d.id)
		  FROM project_decisions d JOIN project_decision_index i ON i.id=d.id WHERE d.curation_id=c.id),'[]'::jsonb))
		|| jsonb_strip_nulls(jsonb_build_object('key',c.key,'decision_id',c.decision_id,'event_id',c.event_id,'source',CASE WHEN c.event_id IS NOT NULL THEN s.source END,'delivery',s.delivery,'goal',s.goal,'document',s.document,'content',CASE WHEN c.document_id IS NOT NULL THEN s.content END))
		FROM material_curations c JOIN curation_sources s ON s.curation_id=c.id WHERE c.project_id=$1`

func (s *Store) MaterialCuration(ctx context.Context, project int64, key string, revision int) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, curationReceipt+" AND c.key=$2 AND c.revision=$3", project, key, revision))
}

func (s *Store) DecisionCuration(ctx context.Context, project, decision int64) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, curationReceipt+" AND c.decision_id=$2", project, decision))
}

func (s *Store) EventCuration(ctx context.Context, project, task, event int64) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, curationReceipt+" AND c.event_id=$2 AND EXISTS (SELECT 1 FROM task_events e WHERE e.id=c.event_id AND e.task_id=$3)", project, event, task))
}

func readCurationReceipt(row *sql.Row) (json.RawMessage, error) {
	var doc json.RawMessage
	err := row.Scan(&doc)
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
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(jsonb_strip_nulls(to_jsonb(a)) ORDER BY id DESC),'[]') FROM (
		SELECT id,key,decision_id,event_id,revision,title,source,kind,receipt_path,content_sha256,facts,reviewed_at
		FROM current_project_experience WHERE project_id=$1 AND ($2::bigint=0 OR id<$2)
		ORDER BY id DESC LIMIT 30) a`, project, before).Scan(&doc)
	return doc, err
}
