package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strconv"
	"time"
)

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("execution report conflicts with the current lease or stored result")
)

//go:embed 001_initial.sql
var initialSchema string

//go:embed 002_local_issues.sql
var issueSchema string

//go:embed 003_delivery.sql
var deliverySchema string

//go:embed 004_inbox.sql
var inboxSchema string

//go:embed 005_plans.sql
var planSchema string

//go:embed 006_plan_archive.sql
var planArchiveSchema string

//go:embed 007_goal_items.sql
var goalItemsSchema string

type Project struct {
	ID           int64  `json:"id"`
	Path         string `json:"path"`
	RepositoryID int64  `json:"repository_id,omitempty"`
}

type Task struct {
	GoalID       int64         `json:"goal_id,omitempty"`
	PlanRevision int           `json:"plan_revision,omitempty"`
	PlanKey      string        `json:"plan_key,omitempty"`
	Children     []GoalChild   `json:"children,omitempty"`
	ID           int64         `json:"id"`
	ProjectID    int64         `json:"project_id"`
	Kind         string        `json:"kind"`
	Spec         *IssueSpec    `json:"spec,omitempty"`
	Status       string        `json:"status"`
	Execution    *Execution    `json:"execution"`
	Attempts     []Execution   `json:"attempts"`
	Events       []Event       `json:"events"`
	Reviews      []Review      `json:"reviews,omitempty"`
	Delivery     *Delivery     `json:"delivery,omitempty"`
	Plan         *PlanRevision `json:"plan,omitempty"`
}

type Execution struct {
	ID           int64      `json:"id"`
	Status       string     `json:"status"`
	Worker       string     `json:"worker"`
	Workspace    string     `json:"workspace"`
	LeaseUntil   time.Time  `json:"lease_until"`
	LeaseExpired bool       `json:"lease_expired"`
	AbandonedAt  *time.Time `json:"abandoned_at,omitempty"`
	EndReason    string     `json:"end_reason,omitempty"`
	Result       *Result    `json:"result"`
}

type Event struct {
	ID          int64     `json:"id"`
	ExecutionID *int64    `json:"execution_id,omitempty"`
	Type        string    `json:"type"`
	At          time.Time `json:"at"`
}

type Job struct {
	Kind        string     `json:"kind"`
	Spec        *IssueSpec `json:"spec,omitempty"`
	Rework      *Review    `json:"rework,omitempty"`
	ExecutionID int64      `json:"execution_id"`
	TaskID      int64      `json:"task_id"`
	RepoPath    string     `json:"repo_path"`
	Workspace   string     `json:"workspace"`
	LeaseToken  string     `json:"lease_token"`
}

type Result struct {
	Commit     string            `json:"commit"`
	ExitCode   int               `json:"exit_code"`
	Error      string            `json:"error"`
	BaseCommit string            `json:"base_commit,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
	Validation *ValidationResult `json:"validation,omitempty"`
}

func (r Result) succeeded() bool {
	return r.ExitCode == 0 && r.Error == "" && r.Commit != ""
}

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize startup across control planes, including first creation of the version table.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(426905306);
		CREATE TABLE IF NOT EXISTS forgeos_schema_versions (version integer PRIMARY KEY)`); err != nil {
		return err
	}
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM forgeos_schema_versions`).Scan(&version); err != nil {
		return err
	}
	migrations := []string{initialSchema, issueSchema, deliverySchema, inboxSchema, planSchema, planArchiveSchema, goalItemsSchema}
	if version < 0 || version > len(migrations) {
		return fmt.Errorf("unsupported database schema version %d", version)
	}
	for version < len(migrations) {
		if _, err = tx.ExecContext(ctx, migrations[version]); err != nil {
			return err
		}
		version++
		if _, err = tx.ExecContext(ctx, `INSERT INTO forgeos_schema_versions VALUES ($1)`, version); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CreateProject(ctx context.Context, path string) (Project, error) {
	var p Project
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO projects(path) VALUES ($1)
		ON CONFLICT(path) DO UPDATE SET path = EXCLUDED.path
		RETURNING id, path, COALESCE(github_repository_id, 0)`, path).Scan(&p.ID, &p.Path, &p.RepositoryID)
	return p, err
}

func (s *Store) CreateTask(ctx context.Context, projectID int64) (Task, error) {
	t := Task{ProjectID: projectID, Kind: "git_head", Status: "READY", Attempts: []Execution{}}
	var e Event
	err := s.db.QueryRowContext(ctx, `
		WITH created AS (
		    INSERT INTO work_items(project_id, status)
		    SELECT id, 'READY' FROM projects WHERE id = $1 RETURNING id
		)
		INSERT INTO task_events(task_id, type) SELECT id, 'task_created' FROM created
		RETURNING task_id, id, type, at`, projectID).Scan(&t.ID, &e.ID, &e.Type, &e.At)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	t.Events = []Event{e}
	return t, err
}

func (s *Store) Task(ctx context.Context, id int64) (Task, error) {
	var t Task
	var spec, attempts, events, reviews, delivery, plan, children []byte
	// A single snapshot keeps the status, execution history and event list consistent.
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.project_id, t.status, t.kind, t.spec,
		    COALESCE((SELECT jsonb_agg(jsonb_build_object(
		        'id', e.id, 'worker', e.worker, 'workspace', e.workspace,
		        'lease_until', e.lease_until, 'lease_expired', e.abandoned_at IS NULL AND e.result IS NULL AND e.lease_until <= now(),
		        'abandoned_at', e.abandoned_at,
		        'end_reason', CASE WHEN e.abandoned_at IS NOT NULL THEN 'owner_abandoned_expired_lease' END,
		        'result', e.result) ORDER BY e.id)
		     FROM executions e WHERE e.task_id = t.id), '[]'),
		    (SELECT jsonb_agg(jsonb_build_object(
		        'id', v.id, 'execution_id', v.execution_id, 'type', v.type, 'at', v.at) ORDER BY v.id)
		     FROM task_events v WHERE v.task_id = t.id),
		    COALESCE((SELECT jsonb_agg(to_jsonb(r) ORDER BY r.execution_id)
		     FROM acceptance_reviews r WHERE r.task_id = t.id), '[]'),
		    (SELECT to_jsonb(d) - 'lease_token' - 'available_at' FROM deliveries d WHERE d.task_id = t.id),
		    (SELECT to_jsonb(r) FROM plan_revisions r WHERE r.task_id=t.id ORDER BY revision DESC LIMIT 1),
		    COALESCE(t.goal_id,0), COALESCE(t.plan_revision,0), COALESCE(t.plan_key,''),
		    (SELECT jsonb_agg(jsonb_build_object('id',c.id,'key',c.plan_key,'status',c.status) ORDER BY c.id)
		       FROM work_items c WHERE c.goal_id=t.id)
		FROM work_items t WHERE t.id = $1`, id).Scan(&t.ID, &t.ProjectID, &t.Status, &t.Kind, &spec, &attempts,
		&events, &reviews, &delivery, &plan, &t.GoalID, &t.PlanRevision, &t.PlanKey, &children)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	if spec != nil {
		if err = json.Unmarshal(spec, &t.Spec); err != nil {
			return Task{}, err
		}
	}
	if plan != nil {
		if err = json.Unmarshal(plan, &t.Plan); err != nil {
			return Task{}, err
		}
	}
	if children != nil {
		if err = json.Unmarshal(children, &t.Children); err != nil {
			return Task{}, err
		}
	}
	if err = json.Unmarshal(attempts, &t.Attempts); err != nil {
		return Task{}, err
	}
	if err = json.Unmarshal(reviews, &t.Reviews); err != nil {
		return Task{}, err
	}
	if delivery != nil {
		if err = json.Unmarshal(delivery, &t.Delivery); err != nil {
			return Task{}, err
		}
	}
	for i := range t.Attempts {
		e := &t.Attempts[i]
		switch {
		case e.AbandonedAt != nil:
			e.Status = "FAILED"
		case e.Result == nil:
			e.Status = "RUNNING"
		case e.Result.succeeded():
			e.Status = "COMPLETED"
		default:
			e.Status = "FAILED"
		}
		t.Execution = e
	}
	err = json.Unmarshal(events, &t.Events)
	return t, err
}

func (s *Store) Claim(ctx context.Context, worker, workspaceRoot string, capabilities ...bool) (*Job, error) {
	if !filepath.IsAbs(workspaceRoot) || worker == "" {
		return nil, errors.New("worker and absolute workspace root required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var j Job
	var projectID int64
	canRunIssue := len(capabilities) > 0 && capabilities[0]
	canRework := len(capabilities) > 1 && capabilities[1]
	err = tx.QueryRowContext(ctx, `
		SELECT p.id, p.path FROM work_items candidate JOIN projects p ON p.id = candidate.project_id
		WHERE candidate.status = 'READY' AND (candidate.kind = 'git_head' OR $1)
		  AND ($2 OR NOT EXISTS (SELECT 1 FROM acceptance_reviews r WHERE r.task_id=candidate.id AND r.decision='reject'))
		  AND NOT EXISTS (
		    SELECT 1 FROM work_items running WHERE running.project_id = p.id AND running.status = 'RUNNING')
		ORDER BY candidate.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, canRunIssue, canRework).Scan(&projectID, &j.RepoPath)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Recheck with a fresh snapshot after taking the project lock.
	var spec []byte
	err = tx.QueryRowContext(ctx, `
		UPDATE work_items SET status = 'RUNNING'
		WHERE id = (SELECT w.id FROM work_items w WHERE w.project_id = $1 AND w.status = 'READY'
		            AND (w.kind = 'git_head' OR $2)
		            AND ($3 OR NOT EXISTS (SELECT 1 FROM acceptance_reviews r WHERE r.task_id=w.id AND r.decision='reject'))
		            ORDER BY w.id LIMIT 1)
		AND NOT EXISTS (SELECT 1 FROM work_items WHERE project_id = $1 AND status = 'RUNNING')
		RETURNING id, kind, spec, nextval(pg_get_serial_sequence('executions','id'))`, projectID, canRunIssue, canRework).Scan(&j.TaskID, &j.Kind, &spec, &j.ExecutionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if spec != nil {
		if err = json.Unmarshal(spec, &j.Spec); err != nil {
			return nil, err
		}
		var review []byte
		err = tx.QueryRowContext(ctx, `SELECT to_jsonb(r) FROM acceptance_reviews r WHERE task_id = $1
			AND decision = 'reject' ORDER BY execution_id DESC LIMIT 1`, j.TaskID).Scan(&review)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if review != nil {
			if err = json.Unmarshal(review, &j.Rework); err != nil {
				return nil, err
			}
		}
	}
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		return nil, err
	}
	j.LeaseToken = hex.EncodeToString(token[:])
	hash := sha256.Sum256([]byte(j.LeaseToken))
	j.Workspace = filepath.Join(workspaceRoot, "execution-"+strconv.FormatInt(j.ExecutionID, 10))
	_, err = tx.ExecContext(ctx, `
		INSERT INTO executions(id, task_id, worker, workspace, lease_hash, lease_until)
		VALUES ($1, $2, $3, $4, $5, now() + CASE WHEN $6 = 'codex_issue'
		    THEN interval '25 minutes' ELSE interval '2 minutes' END)`, j.ExecutionID, j.TaskID, worker, j.Workspace, hash[:], j.Kind)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO task_events(task_id, execution_id, type) VALUES ($1, $2, 'execution_claimed')`, j.TaskID, j.ExecutionID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) Complete(ctx context.Context, id int64, token string, result Result) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var taskID int64
	var abandoned bool
	var expectedHash, stored, spec []byte
	err = tx.QueryRowContext(ctx, `
		SELECT e.task_id, e.lease_hash, e.result, e.abandoned_at IS NOT NULL, t.spec
		FROM executions e JOIN work_items t ON t.id = e.task_id
		WHERE e.id = $1 FOR UPDATE OF e`, id).Scan(&taskID, &expectedHash, &stored, &abandoned, &spec)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(hash[:], expectedHash) != 1 || abandoned {
		return ErrConflict
	}
	if stored != nil {
		var prior Result
		if err = json.Unmarshal(stored, &prior); err != nil {
			return err
		}
		if !reflect.DeepEqual(prior, result) {
			return ErrConflict
		}
		return nil
	}
	if spec != nil {
		var issue IssueSpec
		if err = json.Unmarshal(spec, &issue); err != nil {
			return err
		}
		if !issue.accepts(result) {
			return ErrConflict
		}
		if result.succeeded() {
			var reused bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS (
			    SELECT 1 FROM acceptance_reviews r JOIN executions previous ON previous.id=r.execution_id
			    WHERE r.task_id=$1 AND r.decision='reject' AND (r."commit"=$2 OR previous.result->>'session_id'=$3)
			)`, taskID, result.Commit, result.SessionID).Scan(&reused)
			if err != nil {
				return err
			}
			if reused {
				return ErrConflict
			}
		}
	} else if result.Validation != nil || result.SessionID != "" || result.BaseCommit != "" {
		return ErrConflict
	}
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE executions SET result = $1 WHERE id = $2`, string(data), id); err != nil {
		return err
	}
	status, eventType := "DONE", "execution_completed"
	if !result.succeeded() {
		status, eventType = "FAILED", "execution_failed"
	} else if spec != nil {
		status = "WAITING_ACCEPTANCE"
	}
	return finishTask(ctx, tx, taskID, id, status, eventType)
}

// Abandon releases a project only after its owner reconciles the lost worker.
func (s *Store) Abandon(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var taskID int64
	var abandoned, eligible bool
	err = tx.QueryRowContext(ctx, `
		SELECT task_id, abandoned_at IS NOT NULL, result IS NULL AND lease_until <= now()
		FROM executions WHERE id = $1 FOR UPDATE`, id).Scan(&taskID, &abandoned, &eligible)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil || abandoned {
		return err
	}
	if !eligible {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE executions SET abandoned_at = now() WHERE id = $1`, id); err != nil {
		return err
	}
	return finishTask(ctx, tx, taskID, id, "FAILED", "execution_abandoned")
}

func finishTask(ctx context.Context, tx *sql.Tx, taskID, executionID int64, status, eventType string) error {
	var eventID int64
	err := tx.QueryRowContext(ctx, `
		WITH finished AS (
		    UPDATE work_items SET status = $2 WHERE id = $1 AND status = 'RUNNING' RETURNING id
		)
		INSERT INTO task_events(task_id, execution_id, type) SELECT id, $3, $4 FROM finished
		RETURNING id`, taskID, status, executionID, eventType).Scan(&eventID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
