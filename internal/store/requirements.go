package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type RequirementRevision struct {
	ClassificationID int64            `json:"classification_id,omitempty"`
	Suggestion       *IssueSuggestion `json:"suggestion,omitempty"`
	FeedbackID       int64            `json:"feedback_id,omitempty"`
	FeedbackTask     int64            `json:"feedback_task,omitempty"`
	FeedbackPreview  string           `json:"feedback_preview,omitempty"`
	ID               int64            `json:"id"`
	TaskID           int64            `json:"task_id"`
	Revision         int              `json:"revision"`
	Issue            Issue            `json:"issue"`
	ObservedAt       time.Time        `json:"observed_at"`
}

// The caller holds the imported task lock. Polling a renamed label or a newer branch tip is not a requirement edit.
func recordRequirement(ctx context.Context, tx *sql.Tx, taskID int64, issue Issue, origin string) error {
	var revision int
	var previous []byte
	var seen sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT (SELECT max(revision) FROM requirement_revisions WHERE task_id=$1),r.issue,t.last_issue_update
		FROM requirement_revisions r JOIN work_items t ON t.id=r.task_id WHERE r.task_id=$1 AND r.feedback_id IS NULL
		ORDER BY r.revision DESC LIMIT 1`, taskID).Scan(&revision, &previous, &seen)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var old Issue
	if previous != nil {
		if err = json.Unmarshal(previous, &old); err != nil {
			return err
		}
		if old.ID != issue.ID || old.RepositoryID != issue.RepositoryID || old.Number != issue.Number {
			return ErrConflict
		}
	}
	last := seen.Time
	if !seen.Valid {
		last, _ = time.Parse(time.RFC3339, old.UpdatedAt)
	}
	stamp, parseErr := time.Parse(time.RFC3339, issue.UpdatedAt)
	if stamp.Before(last) {
		return nil
	}
	if parseErr == nil && stamp.After(last) {
		if _, err = tx.ExecContext(ctx, `UPDATE work_items SET last_issue_update=$2 WHERE id=$1`, taskID, stamp); err != nil {
			return err
		}
	}
	if old.WorkType != "" && issue.WorkType == "" {
		issue.WorkType = "clarification"
	}
	if previous != nil && old.Title == issue.Title && old.Body == issue.Body && old.WorkTypeLabels == issue.WorkTypeLabels &&
		(old.WorkType == issue.WorkType || old.OwnerClassified && (issue.OwnerClassified || !ValidWorkType(issue.WorkType))) {
		return nil
	}
	doc, err := json.Marshal(issue)
	if err != nil {
		return err
	}
	var rid int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO requirement_revisions(task_id,revision,issue) VALUES($1,$2,$3) RETURNING id`, taskID, revision+1, string(doc)).Scan(&rid); err != nil {
		return err
	}
	if revision == 0 {
		if issue.ClassificationDigest != "" && issue.OwnerClassified {
			if _, err = tx.ExecContext(ctx, `UPDATE requirement_revisions r SET issue_classification_id=c.id
				FROM work_items w JOIN issue_classifications c ON c.project_id=w.project_id AND c.issue_id=w.github_issue_id
				JOIN discussion_inbox d ON d.project_id=c.project_id AND d.issue_id=c.issue_id AND d.source_digest=c.source_digest
				WHERE r.id=$1 AND w.id=r.task_id AND c.source_digest=$2 AND c.suggestion IS NOT NULL AND d.classification_requested`, rid, issue.ClassificationDigest); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE work_items SET requirement_id=$2 WHERE id=$1`, taskID, rid)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type,origin,requirement_version_id)
			VALUES ($1,'requirement_observed',NULLIF($2,''),$3)`, taskID, origin, rid)
	}
	return err
}

func (s *Store) ObserveIssue(ctx context.Context, projectID int64, issue Issue) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM work_items t JOIN projects p ON p.id=t.project_id WHERE t.project_id=$1 AND t.github_issue_id=$2 AND p.github_repository_id=$3 FOR UPDATE OF t`, projectID, issue.ID, issue.RepositoryID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if err = recordRequirement(ctx, tx, id, issue, "github"); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) RequirementRevision(ctx context.Context, taskID int64, revision int64) (RequirementRevision, error) {
	var r RequirementRevision
	var doc, recommendation []byte
	err := s.db.QueryRowContext(ctx, `SELECT r.id,r.task_id,r.revision,r.issue,r.observed_at,COALESCE(r.feedback_id,0),COALESCE(p.task_id,0),COALESCE(p.id,''),c.suggestion,COALESCE(c.id,0) FROM work_items t
		JOIN requirement_revisions r ON r.task_id=COALESCE(t.goal_id,t.id) LEFT JOIN preview_feedback f ON f.id=r.feedback_id
		LEFT JOIN static_preview_receipts p ON p.id=f.preview_id LEFT JOIN issue_classifications c ON c.id=r.issue_classification_id
		WHERE t.id=$1 AND r.revision=$2`, taskID, revision).Scan(&r.ID, &r.TaskID, &r.Revision, &doc, &r.ObservedAt, &r.FeedbackID, &r.FeedbackTask, &r.FeedbackPreview, &recommendation, &r.ClassificationID)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if recommendation != nil {
		if err = json.Unmarshal(recommendation, &r.Suggestion); err != nil {
			return r, err
		}
	}
	err = json.Unmarshal(doc, &r.Issue)
	return r, err
}

type FeedbackRequirement struct {
	FeedbackID int64  `json:"feedback_id"`
	Execution  int64  `json:"execution_id"`
	Commit     string `json:"commit"`
	Previous   int    `json:"previous_revision"`
	Body       string `json:"body"`
}

func (f FeedbackRequirement) Valid() bool {
	return f.FeedbackID > 0 && f.Execution > 0 && validCommit(f.Commit) && f.Previous > 0 && f.Previous < 2147483647 &&
		strings.TrimSpace(f.Body) != "" && len(f.Body) <= 256<<10 && !strings.ContainsRune(f.Body, 0)
}

// Creating a revision records the Owner's decision. Adoption is a separate checkpoint.
func (s *Store) ReviseFromFeedback(ctx context.Context, task int64, preview string, f FeedbackRequirement) (RequirementRevision, error) {
	if !f.Valid() {
		return RequirementRevision{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RequirementRevision{}, err
	}
	defer tx.Rollback()
	var parent int64
	err = tx.QueryRowContext(ctx, `SELECT parent.id FROM preview_feedback f JOIN static_preview_receipts p ON p.id=f.preview_id
		JOIN work_items child ON child.id=p.task_id JOIN work_items parent ON parent.id=COALESCE(child.goal_id,child.id)
		WHERE f.id=$1 AND p.id=$2 AND p.task_id=$3 AND p.execution_id=$4 AND p.commit=$5
		AND f.classified_at IS NOT NULL AND f.classification->>'category'='requirement_change' FOR UPDATE OF parent`,
		f.FeedbackID, preview, task, f.Execution, f.Commit).Scan(&parent)
	if errors.Is(err, sql.ErrNoRows) {
		return RequirementRevision{}, ErrConflict
	}
	if err != nil {
		return RequirementRevision{}, err
	}
	var revision int
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT revision,revision-1=$2 AND issue->>'body'=$3 FROM requirement_revisions WHERE feedback_id=$1`,
		f.FeedbackID, f.Previous, f.Body).Scan(&revision, &same)
	if err == nil && !same {
		return RequirementRevision{}, ErrConflict
	}
	if errors.Is(err, sql.ErrNoRows) {
		var rid int64
		err = tx.QueryRowContext(ctx, `INSERT INTO requirement_revisions(task_id,revision,issue,feedback_id)
			SELECT task_id,revision+1,jsonb_set(issue,'{body}',to_jsonb($3::text)),$4 FROM requirement_revisions
			WHERE task_id=$1 AND revision=$2 AND issue->>'body'<>$3 AND revision=(SELECT max(revision) FROM requirement_revisions WHERE task_id=$1)
			RETURNING revision,id`, parent, f.Previous, f.Body, f.FeedbackID).Scan(&revision, &rid)
		if errors.Is(err, sql.ErrNoRows) {
			return RequirementRevision{}, ErrConflict
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type,requirement_version_id) VALUES($1,'requirement_revised_from_feedback',$2)`, parent, rid)
		}
	}
	if err != nil {
		return RequirementRevision{}, err
	}
	if err = tx.Commit(); err != nil {
		return RequirementRevision{}, err
	}
	return s.RequirementRevision(ctx, parent, int64(revision))
}

// AdoptRequirement starts new work at a completed turn boundary. Existing executions keep their own requirement IDs.
func (s *Store) AdoptRequirement(ctx context.Context, id, revision int64) (Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var status string
	var bound int64
	var planned bool
	err = tx.QueryRowContext(ctx, `SELECT status,requirement_id,EXISTS(SELECT 1 FROM plan_revisions WHERE task_id=t.id)
		FROM work_items t WHERE id=$1 AND github_issue_id IS NOT NULL FOR UPDATE`, id).Scan(&status, &bound, &planned)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	var newest, rid int64
	var issue []byte
	err = tx.QueryRowContext(ctx, `SELECT revision,id,issue FROM requirement_revisions WHERE task_id=$1 ORDER BY revision DESC LIMIT 1`, id).Scan(&newest, &rid, &issue)
	if err != nil {
		return Task{}, err
	}
	if newest != revision {
		return Task{}, ErrConflict
	}
	if rid != bound {
		if planned || (status != "READY" && status != "WAITING_ACCEPTANCE" && status != "FAILED") {
			return Task{}, ErrConflict
		}
		var next Issue
		if err = json.Unmarshal(issue, &next); err != nil {
			return Task{}, err
		}
		if next.WorkType != "" && !ValidWorkType(next.WorkType) {
			return Task{}, ErrConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE work_items SET requirement_id=$2,
			spec=jsonb_set(spec-'verify_commit','{issue}',$3::jsonb) || CASE
			WHEN $4 AND COALESCE(spec->>'mode','')<>'plan' THEN jsonb_strip_nulls(jsonb_build_object('mode','plan','child_harness',spec->'harness','harness','codex'))
			ELSE '{}'::jsonb END,status='READY' WHERE id=$1`, id, rid, string(issue), next.WorkType != "" && next.WorkType != "development"); err != nil {
			return Task{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type,requirement_version_id) VALUES ($1,'requirement_adopted',$2)`, id, rid); err != nil {
			return Task{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}
