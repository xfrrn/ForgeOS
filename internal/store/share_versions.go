package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/lib/pq"
)

type ShareVersionDraft struct {
	ExpectedCurrentID *int64 `json:"expected_current_id"`
	ExpectedEpoch     *int64 `json:"expected_epoch,omitempty"`
	SourceRevision    int    `json:"source_revision,omitempty"`
	Title             string `json:"title"`
	Content           string `json:"content"`
}

func (v ShareVersionDraft) Valid() bool {
	return v.ExpectedCurrentID != nil && *v.ExpectedCurrentID >= 0 && (v.ExpectedEpoch == nil || *v.ExpectedEpoch >= 0) &&
		v.SourceRevision >= 0 && v.SourceRevision <= math.MaxInt32 && validShareCopy(v.Title, v.Content)
}

func (s *Store) DraftShareVersion(ctx context.Context, project, parent int64, v ShareVersionDraft) (json.RawMessage, error) {
	if parent <= 0 || !v.Valid() {
		return nil, ErrConflict
	}
	return s.createShareVersion(ctx, project, parent, ShareDraft{Title: v.Title, Content: v.Content}, v)
}

func (s *Store) createShareVersion(ctx context.Context, project, parent int64, d ShareDraft, v ShareVersionDraft) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var series, current, epoch int64
	if parent == 0 {
		// Serializes duplicate initial requests without taking locks on both projects.
		_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('share-draft/' || $1::bigint::text,0))`, project)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT g.id,coalesce(g.current_share_id,0),g.publication_epoch,g.recipient,
			coalesce(k.material_key,''),coalesce(k.revision,0),coalesce(k.experience_id,0),coalesce(k.experience_revision,0)
			FROM knowledge_share_series g JOIN knowledge_shares k ON k.series_id=g.id
			WHERE k.project_id=$1 AND k.id=$2 FOR UPDATE OF g`, project, parent).Scan(
			&series, &current, &epoch, &d.Recipient, &d.MaterialKey, &d.Revision, &d.ExperienceID, &d.ExperienceRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if v.SourceRevision > 0 {
			if d.ExperienceID > 0 {
				d.ExperienceRevision = v.SourceRevision
			} else {
				d.Revision = v.SourceRevision
			}
		}
	}
	if err != nil {
		return nil, err
	}
	var digest string
	err = tx.QueryRowContext(ctx, `SELECT encode(sha256(convert_to(jsonb_build_array($1::text,$2::text)::text,'UTF8')),'hex')`, d.Title, d.Content).Scan(&digest)
	if err != nil {
		return nil, err
	}

	var id int64
	if parent == 0 {
		err = tx.QueryRowContext(ctx, `SELECT id FROM knowledge_shares WHERE project_id=$1 AND recipient=$2 AND digest=$3 AND share_version=1
			AND ((material_key,revision)=($4,$5) OR (experience_id,experience_revision)=($6,$7))`,
			project, d.Recipient, digest, d.MaterialKey, d.Revision, d.ExperienceID, d.ExperienceRevision).Scan(&id)
	} else {
		// A lost response may be retried after the draft has already been published.
		// With no explicit epoch, prefer a draft based on the present publication.
		err = tx.QueryRowContext(ctx, `SELECT id FROM knowledge_shares WHERE series_id=$1 AND parent_share_id=$2
			AND digest=$3 AND coalesce(base_share_id,0)=$4 AND coalesce(revision,experience_revision)=$5
			AND base_epoch=coalesce($6::bigint, CASE WHEN $4=$7 THEN $8::bigint ELSE base_epoch END)
			ORDER BY share_version DESC LIMIT 1`, series, parent, digest, *v.ExpectedCurrentID, max(d.Revision, d.ExperienceRevision), v.ExpectedEpoch, current, epoch).Scan(&id)
	}
	if err == nil {
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return s.ShareReceipt(ctx, project, id)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if parent != 0 && (*v.ExpectedCurrentID != current || (v.ExpectedEpoch != nil && *v.ExpectedEpoch != epoch)) {
		return nil, ErrConflict
	}

	var sourceAllowed bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_share_sources WHERE project_id=$1 AND is_effective
		AND ((material_key,revision)=($2,$3) OR (experience_id,experience_revision)=($4,$5))
		AND strpos(lower($6||' '||$7),lower(trim(content)))=0)`, project, d.MaterialKey, d.Revision, d.ExperienceID, d.ExperienceRevision, d.Title, d.Content).Scan(&sourceAllowed)
	if err != nil {
		return nil, err
	}
	if !sourceAllowed {
		return nil, ErrConflict
	}
	if err = tx.QueryRowContext(ctx, `SELECT nextval(pg_get_serial_sequence('knowledge_shares','id'))`).Scan(&id); err != nil {
		return nil, err
	}
	if parent == 0 {
		series = id
		_, err = tx.ExecContext(ctx, `INSERT INTO knowledge_share_series(id,project_id,recipient) VALUES($1,$2,$3)`, series, project, d.Recipient)
		var constraint *pq.Error
		if errors.As(err, &constraint) && constraint.Code == "23503" {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO knowledge_shares(id,project_id,recipient,series_id,parent_share_id,base_share_id,base_epoch,
		material_key,revision,experience_id,experience_revision,title,content,digest,share_version)
		SELECT $1,$2,$3,$4,NULLIF($5::bigint,0),NULLIF($6::bigint,0),$7,NULLIF($8,''),NULLIF($9::int,0),NULLIF($10::bigint,0),
		NULLIF($11::int,0),$12,$13,$14,coalesce(max(share_version),0)+1 FROM knowledge_shares WHERE series_id=$4`,
		id, project, d.Recipient, series, parent, current, epoch, d.MaterialKey, d.Revision, d.ExperienceID, d.ExperienceRevision, d.Title, d.Content, digest)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.ShareReceipt(ctx, project, id)
}

func (s *Store) SetSharePublished(ctx context.Context, project, id int64, digest string, publish bool) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var series, current, epoch int64
	err = tx.QueryRowContext(ctx, `SELECT g.id,coalesce(g.current_share_id,0),g.publication_epoch FROM knowledge_share_series g
		JOIN knowledge_shares k ON k.series_id=g.id WHERE k.project_id=$1 AND k.id=$2 AND k.digest=$3 FOR UPDATE OF g`, project, id, digest).Scan(&series, &current, &epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	switch {
	case publish && current != id:
		var changed int64
		err = tx.QueryRowContext(ctx, `UPDATE knowledge_shares k SET published_at=clock_timestamp() WHERE id=$1
			AND published_at IS NULL AND revoked_at IS NULL AND review->>'verdict'='approve'
			AND base_epoch=$2 AND coalesce(base_share_id,0)=$3
			AND (experience_id IS NULL OR EXISTS(SELECT 1 FROM experience_revision_facts f WHERE f.project_id=k.project_id
			     AND f.entry_id=k.experience_id AND f.revision=k.experience_revision AND f.is_effective)) RETURNING id`, id, epoch, current).Scan(&changed)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConflict
		}
		if err != nil {
			return nil, err
		}
		if current != 0 {
			if _, err = tx.ExecContext(ctx, `UPDATE knowledge_shares SET superseded_at=clock_timestamp() WHERE id=$1`, current); err != nil {
				return nil, err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE knowledge_share_series SET current_share_id=$2,publication_epoch=publication_epoch+1 WHERE id=$1`, series, id)
	case !publish:
		var changed int64
		err = tx.QueryRowContext(ctx, `UPDATE knowledge_shares SET revoked_at=coalesce(revoked_at,clock_timestamp()) WHERE id=$1 AND superseded_at IS NULL RETURNING id`, id).Scan(&changed)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConflict
		}
		if err == nil && current == id {
			_, err = tx.ExecContext(ctx, `UPDATE knowledge_share_series SET current_share_id=NULL,publication_epoch=publication_epoch+1 WHERE id=$1`, series)
		}
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.ShareReceipt(ctx, project, id)
}
