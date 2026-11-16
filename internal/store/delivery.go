package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Review struct {
	ExecutionID int64     `json:"execution_id"`
	Commit      string    `json:"commit"`
	Decision    string    `json:"decision"`
	Note        string    `json:"note"`
	Actor       string    `json:"actor"`
	At          time.Time `json:"at"`
}

type Delivery struct {
	TaskID       int64  `json:"task_id"`
	ExecutionID  int64  `json:"execution_id"`
	Commit       string `json:"commit"`
	Branch       string `json:"branch"`
	TargetBranch string `json:"target_branch"`
	State        string `json:"state"`
	Detail       string `json:"detail,omitempty"`
	PRNumber     int64  `json:"pr_number,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
	MergeSHA     string `json:"merge_sha,omitempty"`
	LeaseToken   string `json:"-"`
}

// Review records a decision on exactly one validated execution. Publication is frozen at approval time.
func (s *Store) Review(ctx context.Context, taskID, executionID int64, commit, decision, note string, publish bool, target string) (Task, error) {
	if !validCommit(commit) || (decision != "approve" && decision != "reject") || (decision == "reject" && strings.TrimSpace(note) == "") {
		return Task{}, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var kind, status string
	var spec []byte
	err = tx.QueryRowContext(ctx, `SELECT kind, status, spec FROM work_items WHERE id=$1 FOR UPDATE`, taskID).Scan(&kind, &status, &spec)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT "commit"=$3 AND decision=$4 AND note=$5 FROM acceptance_reviews
		WHERE task_id=$1 AND execution_id=$2`, taskID, executionID, commit, decision, note).Scan(&same)
	if err == nil {
		if !same {
			return Task{}, ErrConflict
		}
		// A replay after a deployment configuration change must not enqueue another delivery.
		if err = tx.Commit(); err != nil {
			return Task{}, err
		}
		return s.Task(ctx, taskID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Task{}, err
	}
	if kind != "codex_issue" || status != "WAITING_ACCEPTANCE" {
		return Task{}, ErrConflict
	}
	var latest int64
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT id, result FROM executions WHERE task_id=$1 ORDER BY id DESC LIMIT 1`, taskID).Scan(&latest, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrConflict
	}
	if err != nil {
		return Task{}, err
	}
	var issue IssueSpec
	var result Result
	if latest != executionID || raw == nil {
		return Task{}, ErrConflict
	}
	if err = json.Unmarshal(spec, &issue); err != nil {
		return Task{}, err
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return Task{}, err
	}
	if !result.succeeded() || result.Commit != commit || !issue.accepts(result) {
		return Task{}, ErrConflict
	}
	if decision == "approve" {
		if err = canApproveReview(ctx, tx, taskID, executionID); err != nil {
			return Task{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO acceptance_reviews(task_id,execution_id,"commit",decision,note)
		VALUES ($1,$2,$3,$4,$5)`, taskID, executionID, commit, decision, note); err != nil {
		return Task{}, err
	}
	status, event := "READY", "acceptance_rejected"
	if decision == "approve" {
		status, event = "ACCEPTED", "acceptance_approved"
		if publish {
			branch := fmt.Sprintf("forgeos/task-%d-%s", taskID, commit[:12])
			if _, err = tx.ExecContext(ctx, `INSERT INTO deliveries(task_id,execution_id,"commit",branch,target_branch)
				VALUES ($1,$2,$3,$4,$5)`, taskID, executionID, commit, branch, target); err != nil {
				return Task{}, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status=$2,spec=CASE WHEN $2='READY' THEN spec-'verify_commit' ELSE spec END WHERE id=$1`, taskID, status); err != nil {
		return Task{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES ($1,$2,$3)`, taskID, executionID, event); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, taskID)
}

func (s *Store) ClaimDelivery(ctx context.Context, projectID int64) (*Delivery, error) {
	var raw []byte
	var d Delivery
	err := s.db.QueryRowContext(ctx, `
		WITH due AS (
		    SELECT d.task_id FROM deliveries d JOIN work_items t ON t.id=d.task_id
		    WHERE t.project_id=$1 AND t.status='ACCEPTED' AND d.state <> 'DONE' AND d.available_at <= now()
		    ORDER BY d.available_at, d.task_id LIMIT 1 FOR UPDATE OF d SKIP LOCKED
		)
		UPDATE deliveries d SET lease_token=gen_random_uuid(), available_at=now()+interval '2 minutes'
		FROM due WHERE d.task_id=due.task_id
		RETURNING to_jsonb(d) - 'lease_token' - 'available_at', d.lease_token`, projectID).Scan(&raw, &d.LeaseToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// SaveDelivery commits a reconciliation checkpoint and releases this claim. Remote writes are reconciled before retry.
func (s *Store) SaveDelivery(ctx context.Context, d Delivery) error {
	if d.LeaseToken == "" || (d.MergeSHA != "" && !validCommit(d.MergeSHA)) ||
		d.PRNumber < 0 || (d.PRNumber == 0) != (d.PRURL == "") ||
		(d.PRNumber > 0 && d.TargetBranch == "") || ((d.State == "MERGED" || d.State == "DONE") != (d.MergeSHA != "")) ||
		(d.MergeSHA != "" && d.PRNumber == 0) {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var firstPR, firstMerge bool
	err = tx.QueryRowContext(ctx, `
		WITH prior AS (
		    SELECT * FROM deliveries WHERE task_id=$1 AND execution_id=$2 AND "commit"=$3 AND branch=$4
		      AND lease_token::text=$5 AND available_at > now() FOR UPDATE
		)
		UPDATE deliveries d SET state=$6, target_branch=$7, pr_number=$8, pr_url=$9, merge_sha=$10, detail=$11,
		    lease_token=NULL, available_at=now() + CASE
		        WHEN prior.state=$6 AND prior.target_branch=$7 AND prior.pr_number=$8 THEN interval '30 seconds'
		        ELSE interval '0 seconds' END
		FROM prior WHERE d.task_id=prior.task_id
		  AND (prior.target_branch='' OR prior.target_branch=$7)
		  AND (prior.pr_number=0 OR (prior.pr_number=$8 AND prior.pr_url=$9))
		  AND (prior.merge_sha='' OR prior.merge_sha=$10)
		  AND array_position(ARRAY['PENDING','WAITING','MERGED','DONE'], $6) >=
		      array_position(ARRAY['PENDING','WAITING','MERGED','DONE'], prior.state)
		RETURNING prior.pr_number=0 AND d.pr_number>0, prior.merge_sha='' AND d.merge_sha<>''`,
		d.TaskID, d.ExecutionID, d.Commit, d.Branch, d.LeaseToken, d.State, d.TargetBranch, d.PRNumber, d.PRURL, d.MergeSHA, d.Detail).Scan(&firstPR, &firstMerge)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	var events []string
	if firstPR {
		events = append(events, "pr_created")
	}
	if firstMerge {
		events = append(events, "pr_merged")
	}
	if d.State == "DONE" {
		if _, err := tx.ExecContext(ctx, `UPDATE work_items SET status='DONE' WHERE id=$1 AND status='ACCEPTED'`, d.TaskID); err != nil {
			return err
		}
		events = append(events, "delivery_completed")
	}
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `INSERT INTO task_events(task_id,execution_id,type) VALUES ($1,$2,$3)`, d.TaskID, d.ExecutionID, event); err != nil {
			return err
		}
	}
	return tx.Commit()
}
