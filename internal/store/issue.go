package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"forgeos/internal/validation"
)

type Issue struct {
	RepositoryID int64  `json:"repository_id"`
	Repository   string `json:"repository"`
	ID           int64  `json:"id"`
	Number       int64  `json:"number"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	UpdatedAt    string `json:"updated_at"`
}

type IssueSpec struct {
	VerifyCommit string `json:"verify_commit,omitempty"`
	Mode         string `json:"mode,omitempty"`
	Issue        Issue  `json:"issue"`
	BaseCommit   string `json:"base_commit"`
	Validation   string `json:"validation"`
}

type ValidationResult struct {
	Profile  string          `json:"profile"`
	Commit   string          `json:"commit"`
	ExitCode int             `json:"exit_code"`
	Output   string          `json:"output"`
	Run      *validation.Run `json:"run,omitempty"`
}

func ValidValidation(profile string) bool {
	return profile == "docs" || profile == "go-test" || validation.ValidLevel(profile)
}

func (s *Store) Project(ctx context.Context, id int64) (Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx, `SELECT id, path, COALESCE(github_repository_id, 0)
		FROM projects WHERE id = $1`, id).Scan(&p.ID, &p.Path, &p.RepositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (s *Store) ImportIssue(ctx context.Context, projectID int64, spec IssueSpec) (Task, error) {
	issue := spec.Issue
	if spec.VerifyCommit != "" || issue.ID <= 0 || issue.RepositoryID <= 0 || issue.Number <= 0 || !validCommit(spec.BaseCommit) ||
		!ValidValidation(spec.Validation) || (spec.Mode != "" && spec.Mode != "plan") {
		return Task{}, ErrConflict
	}
	snapshot, err := json.Marshal(spec)
	if err != nil {
		return Task{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	if err := bindRepository(ctx, tx, projectID, issue.RepositoryID); err != nil {
		return Task{}, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		WITH created AS (
		    INSERT INTO work_items(project_id, kind, status, github_issue_id, spec)
		    VALUES ($1, 'codex_issue', 'READY', $2, $3)
		    ON CONFLICT (project_id, github_issue_id) DO NOTHING RETURNING id
		)
		INSERT INTO task_events(task_id, type) SELECT id, 'issue_imported' FROM created
		RETURNING task_id`, projectID, issue.ID, string(snapshot)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT id FROM work_items WHERE project_id = $1 AND github_issue_id = $2 FOR UPDATE`,
			projectID, issue.ID).Scan(&id)
	}
	if err != nil {
		return Task{}, err
	}
	if err = recordRequirement(ctx, tx, id, issue); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}

func bindRepository(ctx context.Context, tx *sql.Tx, projectID, repositoryID int64) error {
	var bound sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT github_repository_id FROM projects WHERE id = $1 FOR UPDATE`, projectID).Scan(&bound)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !bound.Valid {
		_, err = tx.ExecContext(ctx, `UPDATE projects SET github_repository_id = $2 WHERE id = $1`, projectID, repositoryID)
		return err
	}
	if bound.Int64 != repositoryID {
		return ErrConflict
	}
	return nil
}

func (s *Store) RetryIssueExecution(ctx context.Context, executionID int64) (Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	var taskID int64
	err = tx.QueryRowContext(ctx, `SELECT task_id FROM executions WHERE id = $1 FOR UPDATE`, executionID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	// Replaying the original request must never schedule another attempt, even if its successor failed.
	var alreadyRetried bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS (
	    SELECT 1 FROM task_events WHERE task_id = $1 AND execution_id = $2 AND type = 'execution_retry_requested'
	)`, taskID, executionID).Scan(&alreadyRetried)
	if err != nil {
		return Task{}, err
	}
	if !alreadyRetried {
		var eventID int64
		err = tx.QueryRowContext(ctx, `
			WITH queued AS (
			    UPDATE work_items SET status = 'READY'
			    WHERE id = $1 AND kind = 'codex_issue' AND status = 'FAILED'
			      AND $2 = (SELECT id FROM executions WHERE task_id = $1 ORDER BY id DESC LIMIT 1)
			    RETURNING id
			)
			INSERT INTO task_events(task_id, execution_id, type)
			SELECT id, $2, 'execution_retry_requested' FROM queued RETURNING id`, taskID, executionID).Scan(&eventID)
		if errors.Is(err, sql.ErrNoRows) {
			return Task{}, ErrConflict
		}
		if err != nil {
			return Task{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, taskID)
}

func validCommit(sha string) bool {
	if len(sha) != 40 && len(sha) != 64 {
		return false
	}
	_, err := hex.DecodeString(sha)
	return err == nil && strings.ToLower(sha) == sha
}

func (spec IssueSpec) accepts(r Result) bool {
	if spec.Mode == "plan" {
		if r.Validation != nil || r.Commit != "" || (r.BaseCommit != "" && r.BaseCommit != spec.BaseCommit) {
			return false
		}
		if r.ExitCode != 0 {
			return r.Plan == nil && strings.TrimSpace(r.Error) != ""
		}
		if r.BaseCommit != spec.BaseCommit {
			return false
		}
		return r.Plan != nil && r.Plan.Validate() == nil && r.Error == "" && strings.TrimSpace(r.SessionID) != ""
	}
	if r.Plan != nil {
		return false
	}
	if r.BaseCommit != "" && r.BaseCommit != spec.BaseCommit {
		return false
	}
	if r.Commit != "" && (!validCommit(r.Commit) || len(r.Commit) != len(spec.BaseCommit) || r.Commit == spec.BaseCommit) {
		return false
	}
	v := r.Validation
	if v != nil && (r.Commit == "" || v.Commit != r.Commit || v.Profile != spec.Validation) {
		return false
	}
	if v != nil && v.Run != nil && (!validation.ValidLevel(spec.Validation) || !v.Run.Valid() ||
		v.Run.Base != spec.BaseCommit || v.Run.Commit != r.Commit || v.Run.Level != spec.Validation || (v.Run.Failure == "") != (v.ExitCode == 0)) {
		return false
	}
	if r.ExitCode != 0 {
		return strings.TrimSpace(r.Error) != ""
	}
	if v != nil && validation.ValidLevel(spec.Validation) {
		if v.Run == nil {
			return false
		}
	}
	validTurn := strings.TrimSpace(r.SessionID) != ""
	if spec.VerifyCommit != "" {
		validTurn = r.SessionID == "" && r.Commit == spec.VerifyCommit
	}
	return r.Error == "" && r.BaseCommit == spec.BaseCommit && validTurn && v != nil && v.ExitCode == 0
}
