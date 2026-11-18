package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type IssueClassificationJob struct {
	ID     int64 `json:"id"`
	Source Issue `json:"source"`
}

type IssueSuggestion struct {
	WorkType  string        `json:"work_type"`
	Rationale string        `json:"rationale"`
	Quote     string        `json:"quote"`
	Session   string        `json:"session_id"`
	Profile   *AgentProfile `json:"profile"`
}

const currentIssueClassification = `EXISTS (SELECT 1 FROM discussion_inbox d WHERE
 d.project_id=c.project_id AND d.issue_id=c.issue_id AND d.source_digest=c.source_digest AND d.classification_requested)
 AND NOT EXISTS (SELECT 1 FROM work_items w WHERE w.project_id=c.project_id AND w.github_issue_id=c.issue_id)`

func (s *Store) ClaimIssueClassification(ctx context.Context, project int64) (*ManagementJob, error) {
	tx, pool, err := s.beginManagementClaim(ctx, project, "issue_classification", true)
	if tx == nil {
		return nil, err
	}
	defer tx.Rollback()
	var source []byte
	var candidate IssueClassificationJob
	job := ManagementJob{ProjectID: project, CaptureProcess: true, IssueClassification: &candidate}
	err = tx.QueryRowContext(ctx, `WITH next AS (SELECT c.id FROM issue_classifications c WHERE c.project_id=$1
 AND c.suggestion IS NULL AND (c.lease_until IS NULL OR c.lease_until<=clock_timestamp()) AND `+currentIssueClassification+`
 ORDER BY c.id LIMIT 1 FOR UPDATE OF c SKIP LOCKED)
 UPDATE issue_classifications c SET lease=gen_random_uuid(),lease_until=clock_timestamp()+interval '6 minutes', model_budget=$2
 FROM next WHERE c.id=next.id RETURNING c.id,c.source,c.lease,c.lease_until`, project, pool).Scan(&candidate.ID, &source, &job.Lease, &job.LeaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(source, &candidate.Source); err != nil {
		return nil, err
	}
	return &job, tx.Commit()
}

func (s *Store) SuggestIssueType(ctx context.Context, project, id int64, lease string, suggestion IssueSuggestion) error {
	if (!ValidWorkType(suggestion.WorkType) && suggestion.WorkType != "clarification") ||
		!decisionText(suggestion.Quote) || !decisionText(suggestion.Rationale) ||
		suggestion.Profile == nil || !validStructuredProfile(suggestion.Profile) || len(suggestion.Session) == 0 || len(suggestion.Session) > 128 ||
		strings.Trim(suggestion.Session, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
		return ErrConflict
	}
	doc, err := json.Marshal(suggestion)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var replay, eligible bool
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(c.suggestion=$4::jsonb,false), c.suggestion IS NULL AND c.lease_until>clock_timestamp()
 AND (strpos(c.source->>'title',$5)>0 OR strpos(c.source->>'body',$5)>0) AND `+currentIssueClassification+`
 FROM issue_classifications c WHERE c.id=$1 AND c.project_id=$2 AND c.lease::text=$3 FOR UPDATE`, id, project, lease, string(doc), suggestion.Quote).Scan(&replay, &eligible)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if replay {
		return nil
	}
	if !eligible {
		return ErrConflict
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{IssueClassificationID: id, Lease: lease}, suggestion.Session); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE issue_classifications SET suggestion=$2 WHERE id=$1`, id, string(doc))
	if err != nil {
		return err
	}
	return tx.Commit()
}
