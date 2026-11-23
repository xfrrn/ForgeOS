package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"forgeos/internal/agents"
	"regexp"
	"strings"

	"forgeos/internal/validation"
)

type Issue struct {
	ClassificationDigest string `json:"-"`
	WorkType             string `json:"work_type,omitempty"`
	OwnerClassified      bool   `json:"owner_classified,omitempty"`
	WorkTypeLabels       string `json:"work_type_labels,omitempty"`
	RepositoryID         int64  `json:"repository_id"`
	Repository           string `json:"repository"`
	ID                   int64  `json:"id"`
	Number               int64  `json:"number"`
	URL                  string `json:"url"`
	Title                string `json:"title"`
	Body                 string `json:"body"`
	UpdatedAt            string `json:"updated_at"`
}

func ValidWorkType(value string) bool {
	switch value {
	case "development", "code-review", "testing", "investigation", "operations":
		return true
	}
	return false
}

type IssueSpec struct {
	FeatureStep        *FeatureStep       `json:"feature_step,omitempty"`
	FeatureIntegration bool               `json:"feature_integration,omitempty"`
	ExplicitHarness    bool               `json:"-"`
	ImportOrigin       string             `json:"-"`
	ProjectConfigBlob  string             `json:"project_config_blob,omitempty"`
	ChildHarness       string             `json:"child_harness,omitempty"`
	Harness            string             `json:"harness,omitempty"`
	VerifyCommit       string             `json:"verify_commit,omitempty"`
	Mode               string             `json:"mode,omitempty"`
	Issue              Issue              `json:"issue"`
	BaseCommit         string             `json:"base_commit"`
	Validation         string             `json:"validation"`
	Policy             *validation.Policy `json:"policy,omitempty"`
}

type ValidationResult struct {
	Profile  string          `json:"profile"`
	Commit   string          `json:"commit"`
	ExitCode int             `json:"exit_code"`
	Output   string          `json:"output"`
	Run      *validation.Run `json:"run,omitempty"`
}

func ValidValidation(profile string) bool {
	return profile == "auto" || profile == "docs" || profile == "go-test" || validation.ValidLevel(profile)
}

func (s *Store) Project(ctx context.Context, id int64) (Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx, `SELECT id, path, COALESCE(github_repository_id, 0), COALESCE(repository_credential_id, 0)
		FROM projects WHERE id = $1`, id).Scan(&p.ID, &p.Path, &p.RepositoryID, &p.RepositoryCredentialID)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

type FeedbackSource struct {
	FeedbackID int64  `json:"feedback_id"`
	TaskID     int64  `json:"task_id"`
	Preview    string `json:"preview"`
	Execution  int64  `json:"execution_id"`
	Commit     string `json:"commit"`
}

const dispatchSourceSQL = ` FROM dispatchable_feedback f WHERE f.project_id=$1 AND f.issue_number<>$2
	AND f.feedback_id=$3 AND f.task_id=$4 AND f.preview=$5 AND f.execution_id=$6 AND f.commit=$7`

// Look up the local receipt before contacting GitHub so retries also work after delivery or closure.
func (s *Store) FeedbackDispatch(ctx context.Context, project, number int64, src FeedbackSource) (*Task, error) {
	var id, priorNumber int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(l.task_id,0),COALESCE((target.spec->'issue'->>'number')::bigint,0)
		FROM (SELECT f.feedback_id`+dispatchSourceSQL+`) f LEFT JOIN feedback_work_items l USING(feedback_id)
		LEFT JOIN work_items target ON target.id=l.task_id`, project, number, src.FeedbackID, src.TaskID, src.Preview, src.Execution, src.Commit).Scan(&id, &priorNumber)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && id != 0 && priorNumber != number) {
		return nil, ErrConflict
	}
	if err != nil || id == 0 {
		return nil, err
	}
	task, err := s.Task(ctx, id)
	return &task, err
}

func (s *Store) ImportIssue(ctx context.Context, projectID int64, spec IssueSpec, sources ...FeedbackSource) (Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	id, err := importIssueTask(ctx, tx, projectID, spec, sources...)
	if err != nil {
		return Task{}, err
	}
	if err = tx.Commit(); err != nil {
		return Task{}, err
	}
	return s.Task(ctx, id)
}

func planningHarness(ctx context.Context, tx *sql.Tx, project int64, workType string) (string, error) {
	key := "planner"
	if workType == "operations" {
		key = "operations"
	}
	d, _ := agents.Lookup(key)
	e, err := agentConfig(ctx, tx, project, d)
	return e.Effective.Harness, err
}

func importIssueTask(ctx context.Context, tx *sql.Tx, projectID int64, spec IssueSpec, sources ...FeedbackSource) (int64, error) {
	issue := spec.Issue
	if spec.FeatureStep != nil || spec.FeatureIntegration || spec.ImportOrigin != "" && spec.ImportOrigin != "owner" && spec.ImportOrigin != "github" ||
		issue.WorkType != "" && !ValidWorkType(issue.WorkType) || issue.OwnerClassified && issue.WorkType == "" ||
		issue.WorkType != "" && issue.WorkType != "development" && spec.Mode != "plan" ||
		len(sources) > 1 || spec.VerifyCommit != "" || issue.ID <= 0 || issue.RepositoryID <= 0 || issue.Number <= 0 || !validCommit(spec.BaseCommit) ||
		!ValidValidation(spec.Validation) || (spec.Validation == "auto") != (spec.Policy != nil) ||
		(spec.Policy != nil && !spec.Policy.Valid()) || (spec.Mode != "" && spec.Mode != "plan") || !spec.ValidHarness() {
		return 0, ErrConflict
	}
	if !spec.ExplicitHarness {
		d, _ := agents.Lookup("implementation")
		entry, err := agentConfig(ctx, tx, projectID, d)
		if err != nil {
			return 0, err
		}
		if h := entry.Effective.Harness; h != "task" {
			if spec.Mode == "plan" {
				spec.ChildHarness = h
			} else {
				spec.Harness = h
			}
		}
	}
	if spec.Mode == "plan" {
		harness, err := planningHarness(ctx, tx, projectID, issue.WorkType)
		if err != nil {
			return 0, err
		}
		if spec.ChildHarness == "" && harness == "claude" {
			spec.ChildHarness = "codex"
		}
		if spec.Harness != "" || harness != "codex" {
			spec.Harness = harness
		}
	}
	snapshot, err := json.Marshal(spec)
	if err != nil {
		return 0, err
	}
	if err := bindRepository(ctx, tx, projectID, issue.RepositoryID); err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `
		WITH created AS (
		    INSERT INTO work_items(project_id, kind, status, github_issue_id, spec)
		    VALUES ($1, 'codex_issue', 'READY', $2, $3)
		    ON CONFLICT (project_id, github_issue_id) DO NOTHING RETURNING id
		)
		INSERT INTO task_events(task_id, type, origin) SELECT id, 'issue_imported', NULLIF($4,'') FROM created
		RETURNING task_id`, projectID, issue.ID, string(snapshot), spec.ImportOrigin).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var frozenWorkType string
		err = tx.QueryRowContext(ctx, `SELECT id,COALESCE(spec->'issue'->>'work_type','') FROM work_items WHERE project_id = $1 AND github_issue_id = $2 FOR UPDATE`,
			projectID, issue.ID).Scan(&id, &frozenWorkType)
		if err == nil && issue.OwnerClassified && frozenWorkType != "" && frozenWorkType != issue.WorkType {
			return 0, ErrConflict
		}
	}
	if err != nil {
		return 0, err
	}
	if err = recordRequirement(ctx, tx, id, issue, spec.ImportOrigin); err != nil {
		return 0, err
	}
	if len(sources) == 1 {
		src := sources[0]
		var linked bool
		err = tx.QueryRowContext(ctx, `WITH source AS (SELECT f.feedback_id`+dispatchSourceSQL+`),
			created AS (INSERT INTO feedback_work_items(feedback_id,task_id) SELECT feedback_id,$8 FROM source
				ON CONFLICT(feedback_id) DO NOTHING RETURNING feedback_id),
			event AS (INSERT INTO task_events(task_id,execution_id,type,origin,feedback_id)
				SELECT $4,$6,'feedback_work_item_dispatched','owner',feedback_id FROM created)
			SELECT EXISTS(SELECT 1 FROM created) OR EXISTS(SELECT 1 FROM source JOIN feedback_work_items USING(feedback_id) WHERE task_id=$8)`,
			projectID, issue.Number, src.FeedbackID, src.TaskID, src.Preview, src.Execution, src.Commit, id).Scan(&linked)
		if err != nil {
			return 0, err
		}
		if !linked {
			return 0, ErrConflict
		}
	}
	return id, nil
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

func (spec IssueSpec) ValidHarness() bool {
	if spec.ProjectConfigBlob != "" && !validCommit(spec.ProjectConfigBlob) {
		return false
	}
	if spec.ChildHarness != "" && (spec.Mode != "plan" || (spec.ChildHarness != "codex" && spec.ChildHarness != "claude")) {
		return false
	}
	return spec.Harness == "" || spec.Harness == "codex" || spec.Harness == "claude"
}

var harnessVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

var codexVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[[:alnum:]-]+(\.[[:alnum:]-]+)*)?(\+[[:alnum:]-]+(\.[[:alnum:]-]+)*)?$`)

func ValidHarnessVersion(harness, version string) bool {
	return len(version) <= 64 && (harness == "claude" && harnessVersion.MatchString(version) || harness == "codex" && codexVersion.MatchString(version))
}

func (spec IssueSpec) accepts(r Result) bool { return spec.acceptsResult(r, false) }
func (spec IssueSpec) acceptsResult(r Result, discussion bool) bool {
	if !discussion && r.DiscussionAnswer != "" {
		return false
	}
	if r.Processes != nil && !r.Processes.matches(r) {
		return false
	}
	if (r.Harness == "") != (r.HarnessVersion == "") || (r.Harness != "" && !ValidHarnessVersion(r.Harness, r.HarnessVersion)) {
		return false
	}
	expected := spec.Harness
	if expected == "" {
		expected = "codex"
	}
	if r.Harness != "" && (r.Harness != expected || spec.VerifyCommit != "") {
		return false
	}
	if spec.Harness == "claude" && (!spec.FeatureIntegration || r.SessionID != "") && spec.VerifyCommit == "" && (r.ExitCode == 0 || r.SessionID != "") && r.Harness != "claude" {
		return false
	}
	if spec.Mode == "plan" {
		if r.Validation != nil || r.Commit != "" || (r.BaseCommit != "" && r.BaseCommit != spec.BaseCommit) {
			return false
		}
		if r.ExitCode != 0 {
			return r.DiscussionAnswer == "" && r.Plan == nil && strings.TrimSpace(r.Error) != ""
		}
		if r.BaseCommit != spec.BaseCommit {
			return false
		}
		if discussion {
			return strings.TrimSpace(r.DiscussionAnswer) != "" && len(r.DiscussionAnswer) <= 32<<10 && !strings.ContainsRune(r.DiscussionAnswer, 0) && (r.Plan == nil || r.Plan.Validate() == nil) && r.Error == "" && strings.TrimSpace(r.SessionID) != ""
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
	if v != nil && v.Run != nil {
		run := v.Run
		if !run.Valid() || run.Base != spec.BaseCommit || run.Commit != r.Commit || (run.Failure == "") != (v.ExitCode == 0) {
			return false
		}
		if spec.Validation == "auto" {
			if run.Decision == nil {
				if run.Level != "auto" || (run.Failure != "CONFIG_FAILURE" && run.Failure != "CANCELLED") || len(run.Checks) != 0 {
					return false
				}
			} else if spec.Policy == nil || run.Decision.Policy != *spec.Policy || run.Decision.Level != run.Level || !validation.ValidLevel(run.Level) {
				return false
			}
		} else if spec.Validation == "go-test" {
			if run.Level != "go-test" || run.ConfigBlob != "" || run.ConfigVersion != 0 || run.Profile != "" || run.Decision != nil || len(run.Checks) != 1 {
				return false
			}
			check := run.Checks[0]
			if check.Name != "go-test" || !check.Required || check.LogDigest == nil || len(check.Outputs) != 0 ||
				check.ExitCode == nil || *check.ExitCode != v.ExitCode || check.Failure != run.Failure {
				return false
			}
		} else if !validation.ValidLevel(spec.Validation) || run.Level != spec.Validation || run.Decision != nil {
			return false
		}
	}
	if r.ExitCode != 0 {
		return strings.TrimSpace(r.Error) != ""
	}
	if v != nil && (validation.ValidLevel(spec.Validation) || spec.Validation == "auto") && v.Run == nil {
		return false
	}
	validTurn := strings.TrimSpace(r.SessionID) != ""
	if spec.FeatureStep != nil {
		return validTurn && r.Error == "" && r.BaseCommit == spec.BaseCommit && r.Commit != "" && v == nil
	}
	if spec.FeatureIntegration && r.SessionID == "" {
		validTurn = r.Harness == ""
	}
	if spec.VerifyCommit != "" {
		validTurn = r.SessionID == "" && r.Commit == spec.VerifyCommit
	}
	return r.Error == "" && r.BaseCommit == spec.BaseCommit && validTurn && v != nil && v.ExitCode == 0
}
