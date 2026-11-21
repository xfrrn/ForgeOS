package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type ExperienceChangeInput struct {
	RequestKey       string `json:"request_key"`
	ExpectedRevision int    `json:"expected_revision"`
	Title            string `json:"title"`
	Source           string `json:"source"`
	Content          string `json:"content"`
	Validity         string `json:"validity"`
	Reason           string `json:"reason"`
}

func (i ExperienceChangeInput) Valid(newEntry bool) bool {
	m := MaterialInput{Kind: "memory", Title: i.Title, Source: i.Source, Content: i.Content}
	return ValidMaterialKey(i.RequestKey) && m.Valid() && i.ExpectedRevision >= 0 && i.ExpectedRevision < 2147483647 &&
		(!newEntry || i.ExpectedRevision == 0) && (newEntry || strings.TrimSpace(i.Reason) != "") &&
		len(i.Reason) <= 2048 && !strings.ContainsRune(i.Reason, 0) && len(i.Validity) <= 1024 && !strings.ContainsRune(i.Validity, 0)
}

func (s *Store) ExperienceEntries(ctx context.Context, project, before int64, status, query string) (json.RawMessage, error) {
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT coalesce(jsonb_agg(to_jsonb(x) ORDER BY id DESC),'[]') FROM (
		SELECT * FROM experience_library l WHERE project_id=$1 AND ($2::bigint=0 OR id<$2)
		AND ($3='all' OR ($3='current' AND status='current') OR ($3='pending' AND has_pending)
		    OR ($3='history' AND (status IN ('withdrawn','stale','rejected') OR current_revision>1 OR EXISTS
		        (SELECT 1 FROM experience_change_states d WHERE d.entry_id=l.id AND d.status IN ('rejected','obsolete','discarded')))))
		AND ($4='' OR strpos(lower(concat_ws(' ',title,statement,quote,source,validity)),lower($4))>0
		    OR EXISTS(SELECT 1 FROM experience_change_states d WHERE d.entry_id=l.id AND ($3 IN('history','all') OR ($3='pending' AND d.status IN ('pending','ready')))
		              AND strpos(lower(concat_ws(' ',d.title,d.source,d.content,d.validity)),lower($4))>0)
		    OR ($3 IN('history','all') AND EXISTS(SELECT 1 FROM experience_revision_facts r WHERE r.entry_id=l.id
		              AND strpos(lower(concat_ws(' ',r.title,r.statement,r.quote,r.validity)),lower($4))>0)))
		ORDER BY id DESC LIMIT 30) x`, project, before, status, query).Scan(&result)
	return result, err
}

func (s *Store) ExperienceEntry(ctx context.Context, project, entry int64) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, `SELECT to_jsonb(l) || jsonb_build_object(
		'revisions',coalesce((SELECT jsonb_agg(to_jsonb(r) ORDER BY revision DESC)
		 FROM experience_revision_facts r WHERE r.project_id=l.project_id AND r.entry_id=l.id),'[]'),
		'changes',coalesce((SELECT jsonb_agg(to_jsonb(d)-'request_key' ORDER BY id DESC)
		 FROM experience_change_states d WHERE d.project_id=l.project_id AND d.entry_id=l.id),'[]'))
		FROM experience_library l WHERE project_id=$1 AND id=$2`, project, entry))
}

func (s *Store) ExperienceChangeCuration(ctx context.Context, project, entry, change int64) (json.RawMessage, error) {
	return readCurationReceipt(s.db.QueryRowContext(ctx, curationReceipt+` AND c.experience_change_id=$3
		AND EXISTS (SELECT 1 FROM experience_changes d WHERE d.id=$3 AND d.entry_id=$2 AND d.project_id=$1)`, project, entry, change))
}

// Mutations for one entry share this lock. In particular adoption, withdrawal
// and submission all compare against the version that is current after locking.
func lockExperienceEntry(ctx context.Context, tx *sql.Tx, project, entry int64) (int, bool, error) {
	var revision int
	var withdrawn bool
	err := tx.QueryRowContext(ctx, `SELECT coalesce(current_revision,0),withdrawn_at IS NOT NULL FROM experience_entries
		WHERE project_id=$1 AND id=$2 FOR UPDATE`, project, entry).Scan(&revision, &withdrawn)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return revision, withdrawn, err
}

func (s *Store) SubmitExperienceChange(ctx context.Context, project, entry int64, i ExperienceChangeInput) (json.RawMessage, error) {
	newEntry := entry == 0
	if !i.Valid(newEntry) {
		return nil, ErrConflict
	}
	if _, err := s.Project(ctx, project); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Serialize retries for new entries before an entry ID exists.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,$2))`, i.RequestKey, project); err != nil {
		return nil, err
	}
	var prior int64
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT entry_id,(new_entry=$3 AND (new_entry OR entry_id=$4)
		AND expected_revision=$5 AND title=$6 AND source=$7 AND content=$8 AND reason=$9 AND validity=$10)
		FROM experience_changes WHERE project_id=$1 AND request_key=$2`, project, i.RequestKey, newEntry, entry, i.ExpectedRevision, i.Title, i.Source, i.Content, i.Reason, i.Validity).Scan(&prior, &same)
	if err == nil {
		if !same {
			return nil, ErrConflict
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return s.ExperienceEntry(ctx, project, prior)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if newEntry {
		err = tx.QueryRowContext(ctx, `INSERT INTO experience_entries(project_id) VALUES($1) RETURNING id`, project).Scan(&entry)
		if err != nil {
			return nil, err
		}
	} else {
		current, withdrawn, err := lockExperienceEntry(ctx, tx, project, entry)
		if err != nil {
			return nil, err
		}
		if withdrawn || current != i.ExpectedRevision {
			return nil, ErrConflict
		}
	}
	var change int64
	err = tx.QueryRowContext(ctx, `INSERT INTO experience_changes(project_id,entry_id,request_key,new_entry,expected_revision,title,source,content,reason,validity)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, project, entry, i.RequestKey, newEntry, i.ExpectedRevision, i.Title, i.Source, i.Content, i.Reason, i.Validity).Scan(&change)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO material_curations(project_id,experience_change_id) VALUES($1,$2)`, project, change)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.ExperienceEntry(ctx, project, entry)
}

func (s *Store) AdoptExperienceChange(ctx context.Context, project, entry, change int64, number int) (json.RawMessage, error) {
	if number < 1 || number > 8 {
		return nil, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, withdrawn, err := lockExperienceEntry(ctx, tx, project, entry)
	if err != nil {
		return nil, err
	}
	var expected, previousNumber, count int
	var curation int64
	var reason, status string
	err = tx.QueryRowContext(ctx, `SELECT d.expected_revision,d.curation_id,d.reason,d.status,jsonb_array_length(d.facts),coalesce(r.fact_number,0)
		FROM experience_change_states d LEFT JOIN experience_revisions r ON r.curation_id=d.curation_id
		WHERE d.project_id=$1 AND d.entry_id=$2 AND d.id=$3`, project, entry, change).Scan(&expected, &curation, &reason, &status, &count, &previousNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if previousNumber != 0 {
		if previousNumber != number {
			return nil, ErrConflict
		}
	} else {
		if withdrawn || expected != current || status != "ready" || number > count {
			return nil, ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO experience_revisions(project_id,entry_id,revision,based_on_revision,curation_id,fact_number,change_reason,actor)
			VALUES($1,$2,$3,$4,$5,$6,$7,'owner')`, project, entry, current+1, current, curation, number, reason)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `UPDATE experience_entries SET current_revision=$2 WHERE id=$1`, entry, current+1)
		if err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.ExperienceEntry(ctx, project, entry)
}

func (s *Store) DiscardExperienceChange(ctx context.Context, project, entry, change int64) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, _, err = lockExperienceEntry(ctx, tx, project, entry); err != nil {
		return nil, err
	}
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM experience_change_states WHERE project_id=$1 AND entry_id=$2 AND id=$3`, project, entry, change).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status == "applied" {
		return nil, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE experience_changes SET discarded_at=coalesce(discarded_at,now()) WHERE id=$1`, change); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.ExperienceEntry(ctx, project, entry)
}

func (s *Store) WithdrawExperience(ctx context.Context, project, entry int64, expected int) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, _, err := lockExperienceEntry(ctx, tx, project, entry)
	if err != nil {
		return nil, err
	}
	if expected != current {
		return nil, ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE experience_entries SET withdrawn_at=coalesce(withdrawn_at,now()) WHERE id=$1`, entry); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.ExperienceEntry(ctx, project, entry)
}
