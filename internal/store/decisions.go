package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type DecisionProposal struct {
	RequestKey   string          `json:"request_key"`
	Statement    string          `json:"statement"`
	Source       string          `json:"source"`
	Validity     string          `json:"validity"`
	ValidUntil   *time.Time      `json:"valid_until"`
	EvidenceRefs []string        `json:"evidence_refs"`
	Supersedes   *int64          `json:"supersedes"`
	Curation     *DecisionOrigin `json:"curation,omitempty"`
}

type DecisionOrigin struct {
	ID   int64 `json:"id"`
	Fact int   `json:"fact"`
}

func (p DecisionProposal) Valid() bool {
	text := func(s string, limit int) bool {
		return strings.TrimSpace(s) != "" && len(s) <= limit && !strings.ContainsRune(s, 0)
	}
	if !text(p.RequestKey, 128) || !text(p.Statement, 4096) || !text(p.Validity, 1024) || (p.Supersedes != nil && *p.Supersedes < 1) {
		return false
	}
	if p.Curation != nil {
		return p.Curation.ID > 0 && p.Curation.Fact >= 1 && p.Curation.Fact <= 8 && p.Source == "" && len(p.EvidenceRefs) == 0
	}
	if !text(p.Source, 1024) || len(p.EvidenceRefs) < 1 || len(p.EvidenceRefs) > 16 {
		return false
	}
	for _, ref := range p.EvidenceRefs {
		if !text(ref, 1024) {
			return false
		}
	}
	return true
}

func (s *Store) ProposeDecision(ctx context.Context, project int64, p DecisionProposal) (json.RawMessage, error) {
	if !p.Valid() {
		return nil, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var originID *int64
	var fact *int
	if origin := p.Curation; origin != nil {
		var digest, session string
		err = tx.QueryRowContext(ctx, `SELECT s.receipt_path,s.content_sha256,c.review->>'session_id'
			FROM material_curations c JOIN curation_sources s ON s.curation_id=c.id
			WHERE c.project_id=$1 AND c.id=$2 AND jsonb_array_length(c.extraction->'facts') >= $3
			AND (c.review->>'verdict'='needs_owner' OR (c.review->>'verdict'='approve' AND c.review->>'level' IN ('D2','D3')))`,
			project, origin.ID, origin.Fact).Scan(&p.Source, &digest, &session)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConflict
		}
		if err != nil {
			return nil, err
		}
		originID, fact = &origin.ID, &origin.Fact
		p.EvidenceRefs = []string{fmt.Sprintf("curation:%d/fact:%d", origin.ID, origin.Fact), "sha256:" + digest, "management:" + session}
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO project_decisions AS d
		(project_id,request_key,statement,source,validity,valid_until,evidence_refs,supersedes,curation_id,curation_fact)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (project_id,request_key) DO UPDATE SET request_key=EXCLUDED.request_key
		WHERE (d.statement,d.source,d.validity,d.valid_until,d.evidence_refs,d.supersedes,d.curation_id,d.curation_fact) IS NOT DISTINCT FROM
		(EXCLUDED.statement,EXCLUDED.source,EXCLUDED.validity,EXCLUDED.valid_until,EXCLUDED.evidence_refs,EXCLUDED.supersedes,EXCLUDED.curation_id,EXCLUDED.curation_fact)
		RETURNING id`, project, p.RequestKey, p.Statement, p.Source, p.Validity, p.ValidUntil, pq.Array(p.EvidenceRefs), p.Supersedes, originID, fact).Scan(&id)
	var constraint *pq.Error
	if errors.As(err, &constraint) {
		switch constraint.Code {
		case "23503":
			return nil, ErrNotFound
		case "23514":
			return nil, ErrConflict
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_decision_events (decision_id,status,actor) VALUES ($1,'PROPOSED','owner') ON CONFLICT DO NOTHING`, id)
	if err != nil {
		return nil, err
	}
	return commitDecision(ctx, tx, id)
}

func commitDecision(ctx context.Context, tx *sql.Tx, id int64) (json.RawMessage, error) {
	var result json.RawMessage
	if err := tx.QueryRowContext(ctx, `SELECT to_jsonb(d) FROM project_decision_index d WHERE d.id=$1`, id).Scan(&result); err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func (s *Store) Decide(ctx context.Context, project, id int64, next string) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Serialize confirmations for a project so two proposals cannot both replace the same decision.
	var locked int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE id=$1 FOR UPDATE`, project).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var current string
	var previous sql.NullInt64
	var recorded bool
	err = tx.QueryRowContext(ctx, `SELECT status,supersedes,EXISTS(SELECT 1 FROM project_decision_events WHERE decision_id=$2 AND status=$3)
		FROM project_decision_index WHERE project_id=$1 AND id=$2`, project, id, next).Scan(&current, &previous, &recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	switch next {
	case "CONFIRMED", "REJECTED", "EXPIRED":
	default:
		return nil, ErrConflict
	}
	if recorded || next == current {
		return commitDecision(ctx, tx, id)
	}
	if next == "EXPIRED" && current != "CONFIRMED" || next != "EXPIRED" && current != "PROPOSED" {
		return nil, ErrConflict
	}
	if next == "CONFIRMED" && previous.Valid {
		var retired int64
		err = tx.QueryRowContext(ctx, `INSERT INTO project_decision_events(decision_id,status,actor)
			SELECT id,'SUPERSEDED','owner' FROM project_decision_index WHERE id=$1 AND project_id=$2 AND status='CONFIRMED'
			RETURNING decision_id`, previous.Int64, project).Scan(&retired)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConflict
		}
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO project_decision_events(decision_id,status,actor) VALUES ($1,$2,'owner')`, id, next)
	if err != nil {
		return nil, err
	}
	return commitDecision(ctx, tx, id)
}

func (s *Store) Decisions(ctx context.Context, project, before int64, status string) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(d) ORDER BY d.id DESC),'[]') FROM
		(SELECT * FROM project_decision_index WHERE project_id=$1 AND ($2::bigint=0 OR id<$2)
		 AND ($3='' OR status=$3) ORDER BY id DESC LIMIT 30) d`, project, before, status).Scan(&result)
	return result, err
}

func (s *Store) Decision(ctx context.Context, project, id int64) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, "SELECT to_jsonb(d) FROM project_decision_index d WHERE project_id=$1 AND id=$2", project, id).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

// Follow the replacement chain in both directions, including pending alternatives.
// The plan reference still identifies the exact decision named in the plan.
func (s *Store) DecisionWithVersions(ctx context.Context, project, id int64) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, `WITH RECURSIVE ancestors AS (
		SELECT id,supersedes FROM project_decisions WHERE project_id=$1 AND id=$2
		UNION ALL SELECT d.id,d.supersedes FROM project_decisions d JOIN ancestors a ON d.id=a.supersedes WHERE d.project_id=$1
	), family(id) AS (
		SELECT id FROM ancestors WHERE supersedes IS NULL
		UNION ALL SELECT d.id FROM project_decisions d JOIN family f ON d.supersedes=f.id WHERE d.project_id=$1
	) SELECT to_jsonb(d)||jsonb_build_object('versions',(SELECT jsonb_agg(jsonb_build_object(
		'id',v.id,'status',v.status,'statement',v.statement,'supersedes',v.supersedes) ORDER BY v.id DESC)
		FROM family f JOIN project_decision_index v ON f.id=v.id))
		FROM project_decision_index d WHERE d.project_id=$1 AND d.id=$2`, project, id).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return result, err
}
