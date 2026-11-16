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
	"reflect"
	"strconv"
	"time"

	"github.com/lib/pq"
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

//go:embed 008_questions.sql
var questionSchema string

//go:embed 009_goal_closure.sql
var goalClosureSchema string

//go:embed 010_management.sql
var managementSchema string

//go:embed 011_rolling.sql
var rollingSchema string

//go:embed 012_requirements.sql
var requirementSchema string

//go:embed 013_comments.sql
var commentSchema string

//go:embed 014_checkpoints.sql
var checkpointSchema string

//go:embed 015_delivery_refresh.sql
var refreshSchema string

//go:embed 016_decisions.sql
var decisionSchema string

//go:embed 017_materials.sql
var materialSchema string

//go:embed 018_shares.sql
var shareSchema string

//go:embed 019_secret_metadata.sql
var secretSchema string

//go:embed 020_secret_grants.sql
var secretGrantsSchema string

//go:embed 021_code_reviews.sql
var codeReviewSchema string

//go:embed 022_commit_status.sql
var commitStatusSchema string

//go:embed 023_test_plans.sql
var testPlanTables string

//go:embed 024_test_case_results.sql
var testCaseTables string

//go:embed 025_previews.sql
var previewTables string

//go:embed 026_preview_feedback.sql
var feedbackTables string

//go:embed 027_feedback_classification.sql
var feedbackClassificationTables string

//go:embed 028_feedback_questions.sql
var feedbackQuestionsTable string

//go:embed 029_feedback_reviews.sql
var feedbackReviewsTable string

//go:embed 030_feedback_requirements.sql
var feedbackRequirementsTable string

//go:embed 031_feedback_work_items.sql
var feedbackWorkItemsTable string

//go:embed 032_feedback_artifacts.sql
var feedbackArtifactsTable string

//go:embed 033_worker_nodes.sql
var workerNodesTable string

//go:embed 034_execution_resources.sql
var executionResourcesSchema string

//go:embed 035_ci_retention.sql
var ciRetentionSchema string

//go:embed 036_screenshot_retention.sql
var screenshotRetentionSchema string

//go:embed 037_retention_history.sql
var retentionHistorySchema string

//go:embed 038_github_events.sql
var githubEventsSchema string

//go:embed 039_issue_closures.sql
var issueClosuresSchema string

//go:embed 040_project_checks.sql
var projectChecksSchema string

//go:embed 041_repository_inventories.sql
var inventoriesSchema string

type Project struct {
	ID           int64  `json:"id"`
	Path         string `json:"path"`
	RepositoryID int64  `json:"repository_id,omitempty"`
}

type Task struct {
	SourceClosure       json.RawMessage    `json:"source_closure,omitempty"`
	FeedbackSources     []FeedbackSource   `json:"feedback_sources,omitempty"`
	RequiresCodeReview  bool               `json:"requires_code_review,omitempty"`
	Refreshes           []DeliveryRefresh  `json:"delivery_refreshes,omitempty"`
	RequirementRevision int                `json:"requirement_revision,omitempty"`
	LatestRequirement   int                `json:"latest_requirement,omitempty"`
	ActiveRevision      int                `json:"active_revision,omitempty"`
	Questions           []DecisionQuestion `json:"questions,omitempty"`
	GoalID              int64              `json:"goal_id,omitempty"`
	PlanRevision        int                `json:"plan_revision,omitempty"`
	PlanKey             string             `json:"plan_key,omitempty"`
	Children            []GoalChild        `json:"children,omitempty"`
	ID                  int64              `json:"id"`
	ProjectID           int64              `json:"project_id"`
	Kind                string             `json:"kind"`
	Spec                *IssueSpec         `json:"spec,omitempty"`
	Status              string             `json:"status"`
	Execution           *Execution         `json:"execution"`
	Attempts            []Execution        `json:"attempts"`
	Events              []Event            `json:"events"`
	Reviews             []Review           `json:"reviews,omitempty"`
	Delivery            *Delivery          `json:"delivery,omitempty"`
	Plan                *PlanRevision      `json:"plan,omitempty"`
}

type Execution struct {
	ReviewFeedback      []ReviewFeedback `json:"review_feedback,omitempty"`
	TestCaseState       string           `json:"test_case_state,omitempty"`
	TestCaseAttempts    int64            `json:"test_case_attempts,omitempty"`
	RequiresTestPlan    bool             `json:"requires_test_plan,omitempty"`
	TestPlanState       string           `json:"test_plan_state,omitempty"`
	TestPlanRevision    int              `json:"test_plan_revision,omitempty"`
	CommitStatus        *CommitStatus    `json:"commit_status,omitempty"`
	CodeReviewState     string           `json:"code_review_state,omitempty"`
	UsesCheckpoints     bool             `json:"uses_checkpoints,omitempty"`
	RequirementID       int64            `json:"requirement_id,omitempty"`
	RequirementRevision int              `json:"requirement_revision,omitempty"`
	ID                  int64            `json:"id"`
	Status              string           `json:"status"`
	Worker              string           `json:"worker"`
	Workspace           string           `json:"workspace"`
	LeaseUntil          time.Time        `json:"lease_until"`
	LeaseExpired        bool             `json:"lease_expired"`
	AbandonedAt         *time.Time       `json:"abandoned_at,omitempty"`
	EndReason           string           `json:"end_reason,omitempty"`
	Result              *Result          `json:"result"`
}

type Event struct {
	ID          int64     `json:"id"`
	ExecutionID *int64    `json:"execution_id,omitempty"`
	Type        string    `json:"type"`
	At          time.Time `json:"at"`
}

type Job struct {
	ManageResources     bool       `json:"manage_resources,omitempty"`
	RecordProcesses     bool       `json:"record_processes,omitempty"`
	LeaseSeconds        int        `json:"lease_seconds,omitempty"`
	SupportsCheckpoints bool       `json:"supports_checkpoints,omitempty"`
	Kind                string     `json:"kind"`
	Spec                *IssueSpec `json:"spec,omitempty"`
	Rework              *Review    `json:"rework,omitempty"`
	ExecutionID         int64      `json:"execution_id"`
	TaskID              int64      `json:"task_id"`
	RepoPath            string     `json:"repo_path"`
	RepoRelativePath    string     `json:"repo_relative_path,omitempty"`
	Workspace           string     `json:"workspace"`
	LeaseToken          string     `json:"lease_token"`
}

type Result struct {
	Processes      *ProcessHistory   `json:"processes,omitempty"`
	Harness        string            `json:"harness,omitempty"`
	HarnessVersion string            `json:"harness_version,omitempty"`
	Plan           *Plan             `json:"plan,omitempty"`
	Commit         string            `json:"commit"`
	ExitCode       int               `json:"exit_code"`
	Error          string            `json:"error"`
	BaseCommit     string            `json:"base_commit,omitempty"`
	SessionID      string            `json:"session_id,omitempty"`
	Validation     *ValidationResult `json:"validation,omitempty"`
}

func (r Result) succeeded() bool {
	return r.ExitCode == 0 && r.Error == "" && (r.Commit != "" || r.Plan != nil)
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
	migrations := []string{initialSchema, issueSchema, deliverySchema, inboxSchema, planSchema, planArchiveSchema, goalItemsSchema, questionSchema, goalClosureSchema, managementSchema, rollingSchema, requirementSchema, commentSchema, checkpointSchema, refreshSchema, decisionSchema, materialSchema, shareSchema, secretSchema, secretGrantsSchema, codeReviewSchema, commitStatusSchema, testPlanTables, testCaseTables, previewTables, feedbackTables, feedbackClassificationTables, feedbackQuestionsTable, feedbackReviewsTable, feedbackRequirementsTable, feedbackWorkItemsTable, feedbackArtifactsTable, workerNodesTable, executionResourcesSchema, ciRetentionSchema, screenshotRetentionSchema, retentionHistorySchema, githubEventsSchema, issueClosuresSchema, projectChecksSchema, inventoriesSchema}
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
	return createHeadTask(ctx, s.db, projectID)
}

func createHeadTask(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, projectID int64) (Task, error) {
	t := Task{ProjectID: projectID, Kind: "git_head", Status: "READY", Attempts: []Execution{}}
	var e Event
	err := db.QueryRowContext(ctx, `
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
	var spec, attempts, events, reviews, delivery, plan, children, questions, refreshes, feedback, closure []byte
	// A single snapshot keeps the status, execution history and event list consistent.
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.project_id, p.requires_code_review, t.status, t.kind, COALESCE(t.active_revision,0),COALESCE(frozen.revision,0),COALESCE((SELECT MAX(revision) FROM requirement_revisions WHERE task_id=COALESCE(t.goal_id,t.id)),0),t.spec,
		    COALESCE((SELECT jsonb_agg(jsonb_build_object(
		        'id', e.id,'uses_checkpoints',e.uses_checkpoints, 'requirement_id',e.requirement_id,'requirement_revision',er.revision,'worker', e.worker, 'workspace', e.workspace,
		        'lease_until', e.lease_until, 'lease_expired', e.abandoned_at IS NULL AND e.result IS NULL AND e.lease_until <= now(),
		        'abandoned_at', e.abandoned_at,
		        'review_feedback',(SELECT jsonb_agg(jsonb_build_object('id',f.id,'preview_id',f.preview_id,'decision',f.decision,'note',f.note) ORDER BY f.id)
		            FROM candidate_review_feedback f WHERE f.execution_id=e.id AND t.status='WAITING_ACCEPTANCE'
		            AND e.id=(SELECT max(id) FROM executions WHERE task_id=t.id)),
		        'end_reason', CASE WHEN e.abandoned_at IS NOT NULL THEN 'owner_abandoned_expired_lease' END,
		        'result', e.result,'commit_status',e.commit_status, 'test_plan_state',tp.state,'test_plan_revision',tp.revision,'test_case_state',tp.execution_state,'test_case_attempts',tp.attempt_count, 'code_review_state',`+codeReviewState+`) ORDER BY e.id)
		     FROM executions e LEFT JOIN requirement_revisions er ON er.id=e.requirement_id
		     LEFT JOIN LATERAL (SELECT state,revision,execution_state,attempt_count FROM candidate_test_plan_status WHERE execution_id=e.id ORDER BY revision DESC LIMIT 1) tp ON true
		     LEFT JOIN code_reviews r ON r.execution_id=e.id WHERE e.task_id = t.id), '[]'),
		    (SELECT jsonb_agg(jsonb_build_object(
		        'id', v.id, 'execution_id', v.execution_id, 'type', v.type, 'at', v.at) ORDER BY v.id)
		     FROM task_events v WHERE v.task_id = t.id),
		    COALESCE((SELECT jsonb_agg(to_jsonb(r) || jsonb_build_object('feedback_ids',
		        ARRAY(SELECT feedback_id FROM acceptance_review_feedback WHERE execution_id=r.execution_id ORDER BY feedback_id)) ORDER BY r.execution_id)
		     FROM acceptance_reviews r WHERE r.task_id = t.id), '[]'),
		    (SELECT to_jsonb(d) - 'lease_token' - 'available_at' FROM deliveries d WHERE d.task_id = t.id),
		    (SELECT to_jsonb(r) FROM plan_revisions r WHERE r.task_id=t.id ORDER BY revision DESC LIMIT 1),
		    COALESCE(t.goal_id,0), COALESCE(t.plan_revision,0), COALESCE(t.plan_key,''),
		    (SELECT jsonb_agg(jsonb_build_object('id',c.id,'key',c.plan_key,'status',c.status) ORDER BY c.id)
		       FROM work_items c WHERE c.goal_id=t.id),
		    (SELECT jsonb_agg(to_jsonb(q) ORDER BY q.id) FROM decision_questions q WHERE q.task_id=t.id),
		    (SELECT jsonb_agg(to_jsonb(f) ORDER BY f.execution_id) FROM delivery_refreshes f WHERE f.task_id=t.id),
		    (SELECT jsonb_agg(to_jsonb(f) ORDER BY f.feedback_id) FROM feedback_work_items l JOIN dispatchable_feedback f USING(feedback_id) WHERE l.task_id=t.id),
		    (SELECT to_jsonb(c) FROM github_issue_closures c WHERE c.task_id=t.source_closure_id)
		FROM work_items t JOIN projects p ON p.id=t.project_id LEFT JOIN requirement_revisions frozen ON frozen.id=t.requirement_id WHERE t.id = $1`, id).Scan(&t.ID, &t.ProjectID, &t.RequiresCodeReview, &t.Status, &t.Kind, &t.ActiveRevision, &t.RequirementRevision, &t.LatestRequirement, &spec, &attempts,
		&events, &reviews, &delivery, &plan, &t.GoalID, &t.PlanRevision, &t.PlanKey, &children, &questions, &refreshes, &feedback, &closure)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	t.SourceClosure = closure
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
	if refreshes != nil {
		if err = json.Unmarshal(refreshes, &t.Refreshes); err != nil {
			return Task{}, err
		}
	}
	if questions != nil {
		if err = json.Unmarshal(questions, &t.Questions); err != nil {
			return Task{}, err
		}
	}
	if feedback != nil {
		if err = json.Unmarshal(feedback, &t.FeedbackSources); err != nil {
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
		e.RequiresTestPlan = e.Result.manualPlanRequired()
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
	enabled := func(i int) bool { return i < len(capabilities) && capabilities[i] }
	return s.ClaimForProjects(ctx, worker, workspaceRoot, nil, WorkerCapabilities{
		SupportsCodex: enabled(0), SupportsRework: enabled(1), SupportsPlanner: enabled(2), SupportsCheckpoints: enabled(3),
		SupportsValidation: enabled(4), SupportsDeclaredChecks: enabled(5), SupportsRiskPolicy: enabled(6), SupportsClaude: enabled(7),
	})
}

func (s *Store) WorkerOwnsExecution(ctx context.Context, id int64, worker string, projects []int64) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.worker=$2 AND t.project_id=ANY($3::bigint[]))`, id, worker, pq.Array(projects)).Scan(&allowed)
	return allowed, err
}

func (s *Store) ClaimForProjects(ctx context.Context, worker, workspaceRoot string, projects []int64, caps WorkerCapabilities) (*Job, error) {
	if !ValidWorkspaceRoot(workspaceRoot) || worker == "" {
		return nil, errors.New("worker and absolute workspace root required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var j Job
	var projectID int64
	// Serialize reservations across projects, including simultaneous claims by the same node.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('forgeos.worker_capacity'),hashtext($1))`, worker); err != nil {
		return nil, err
	}
	var available bool
	var registered []byte
	err = tx.QueryRowContext(ctx, `SELECT used < COALESCE(node.slots,1), node.capabilities
		FROM (SELECT count(*) AS used FROM executions WHERE worker=$1 AND result IS NULL AND abandoned_at IS NULL) active
		LEFT JOIN (SELECT capabilities, CASE WHEN last_seen > now() - interval '45 seconds' THEN slots ELSE 0 END AS slots
		           FROM worker_nodes WHERE worker=$1 FOR UPDATE) node ON true`, worker).Scan(&available, &registered)
	if err != nil || !available {
		return nil, err
	}
	if registered != nil {
		var node WorkerCapabilities
		if err := json.Unmarshal(registered, &node); err != nil {
			return nil, err
		}
		caps = caps.intersect(node)
		if node.SupportsResources && !caps.SupportsResources {
			// The registered node requires model and CI reservations for Issue jobs.
			caps.SupportsCodex, caps.SupportsClaude, caps.SupportsValidation = false, false, false
		}
	}
	canAssess := caps.SupportsDeclaredChecks && caps.SupportsRiskPolicy
	err = tx.QueryRowContext(ctx, `
		SELECT p.id, p.path FROM work_items candidate JOIN projects p ON p.id = candidate.project_id
		WHERE ($6::bigint[] IS NULL OR p.id=ANY($6::bigint[])) AND candidate.status = 'READY' AND (candidate.goal_id IS NULL OR EXISTS (SELECT 1 FROM work_items g WHERE g.id=candidate.goal_id AND g.status='ACTIVE'))
		  AND (candidate.kind = 'git_head' OR ($5 AND candidate.spec ? 'verify_commit') OR CASE WHEN candidate.spec->>'harness'='claude' THEN $9::boolean ELSE $1 END)
		  AND ($5 OR NOT COALESCE(candidate.spec ? 'verify_commit',false))
		  AND ($7 OR COALESCE(candidate.spec->>'validation','') NOT IN ('V1','V2','V3','V4'))
		  AND ($8 OR candidate.spec->>'validation' IS DISTINCT FROM 'auto')
		  AND ($4 OR candidate.spec ? 'verify_commit' OR NOT EXISTS (SELECT 1 FROM issue_comment_events c WHERE c.task_id=COALESCE(candidate.goal_id,candidate.id)))
		  AND ($3 OR candidate.spec->>'mode' IS DISTINCT FROM 'plan')
		  AND ($2 OR candidate.spec ? 'verify_commit' OR NOT EXISTS (SELECT 1 FROM acceptance_reviews r JOIN executions e ON e.id=r.execution_id WHERE r.task_id=candidate.id AND r.decision='reject' AND e.requirement_id IS NOT DISTINCT FROM candidate.requirement_id))
		  AND NOT EXISTS (
		    SELECT 1 FROM work_items running WHERE running.project_id = p.id AND running.status = 'RUNNING')
		ORDER BY candidate.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, caps.SupportsCodex, caps.SupportsRework, caps.SupportsPlanner, caps.SupportsCheckpoints, caps.SupportsValidation, pq.Array(projects), caps.SupportsDeclaredChecks, canAssess, caps.SupportsClaude).Scan(&projectID, &j.RepoPath)
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
		WHERE status = 'READY' AND id = (SELECT w.id FROM work_items w WHERE w.project_id = $1 AND w.status = 'READY'
		            AND (w.goal_id IS NULL OR EXISTS (SELECT 1 FROM work_items g WHERE g.id=w.goal_id AND g.status='ACTIVE'))
		            AND (w.kind = 'git_head' OR ($6 AND w.spec ? 'verify_commit') OR CASE WHEN w.spec->>'harness'='claude' THEN $9::boolean ELSE $2 END)
		            AND ($6 OR NOT COALESCE(w.spec ? 'verify_commit',false))
		            AND ($7 OR COALESCE(w.spec->>'validation','') NOT IN ('V1','V2','V3','V4'))
		            AND ($8 OR w.spec->>'validation' IS DISTINCT FROM 'auto')
		            AND ($5 OR w.spec ? 'verify_commit' OR NOT EXISTS (SELECT 1 FROM issue_comment_events c WHERE c.task_id=COALESCE(w.goal_id,w.id)))
		            AND ($4 OR w.spec->>'mode' IS DISTINCT FROM 'plan')
		            AND ($3 OR w.spec ? 'verify_commit' OR NOT EXISTS (SELECT 1 FROM acceptance_reviews r JOIN executions e ON e.id=r.execution_id WHERE r.task_id=w.id AND r.decision='reject' AND e.requirement_id IS NOT DISTINCT FROM w.requirement_id))
		            ORDER BY w.id LIMIT 1)
		AND NOT EXISTS (SELECT 1 FROM work_items WHERE project_id = $1 AND status = 'RUNNING')
		RETURNING id, kind, spec, nextval(pg_get_serial_sequence('executions','id'))`, projectID, caps.SupportsCodex, caps.SupportsRework, caps.SupportsPlanner, caps.SupportsCheckpoints, caps.SupportsValidation, caps.SupportsDeclaredChecks, canAssess, caps.SupportsClaude).Scan(&j.TaskID, &j.Kind, &spec, &j.ExecutionID)
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
		j.SupportsCheckpoints = caps.SupportsCheckpoints && j.Spec.VerifyCommit == ""
		j.RecordProcesses = j.Spec.VerifyCommit == ""
		j.ManageResources = registered != nil && caps.SupportsResources
		var review []byte
		err = tx.QueryRowContext(ctx, `SELECT to_jsonb(r) FROM acceptance_reviews r JOIN executions e ON e.id=r.execution_id JOIN work_items t ON t.id=r.task_id WHERE r.task_id=$1
			AND r.decision='reject' AND e.requirement_id IS NOT DISTINCT FROM t.requirement_id ORDER BY r.execution_id DESC LIMIT 1`, j.TaskID).Scan(&review)
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
	j.Workspace = workerWorkspace(workspaceRoot, "execution-"+strconv.FormatInt(j.ExecutionID, 10))
	j.LeaseSeconds = executionLeaseSeconds(j.Kind)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO executions(id, task_id, worker, workspace, lease_hash, requirement_id, uses_checkpoints, resource_managed, lease_until)
		VALUES ($1, $2, $3, $4, $5, (SELECT requirement_id FROM work_items WHERE id=$2),$7,$8,
		    clock_timestamp() + $6 * interval '1 second')`, j.ExecutionID, j.TaskID, worker, j.Workspace, hash[:], j.LeaseSeconds, j.SupportsCheckpoints, j.ManageResources)
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
	return s.CompleteWithChecks(ctx, id, token, result, nil)
}

type CheckVerifier func(context.Context, string, IssueSpec, Result) (bool, error)

func (s *Store) CompleteWithChecks(ctx context.Context, id int64, token string, result Result, verify CheckVerifier) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var taskID int64
	var abandoned, usesCheckpoints, sourceClosed bool
	var expectedHash, stored, spec []byte
	var repo, worker string
	err = tx.QueryRowContext(ctx, `
		SELECT e.task_id, e.lease_hash, e.result, e.abandoned_at IS NOT NULL, t.spec,e.uses_checkpoints,p.path,e.worker,t.source_closure_id IS NOT NULL
		FROM executions e JOIN work_items t ON t.id = e.task_id JOIN projects p ON p.id=t.project_id
		WHERE e.id = $1 FOR UPDATE OF e,t`, id).Scan(&taskID, &expectedHash, &stored, &abandoned, &spec, &usesCheckpoints, &repo, &worker, &sourceClosed)
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
	if spec == nil && result.Processes != nil {
		return ErrConflict
	}
	if spec != nil {
		var issue IssueSpec
		if err = json.Unmarshal(spec, &issue); err != nil {
			return err
		}
		if !issue.accepts(result) {
			return ErrConflict
		}
		if v := result.Validation; v != nil && v.Run != nil && (v.Run.ExecutionID != id || v.Run.Runner != worker) {
			return ErrConflict
		}
		if result.succeeded() && !sourceClosed {
			if v := result.Validation; v != nil && v.Run != nil {
				if verify == nil {
					return ErrConflict
				}
				changed, err := verify(ctx, repo, issue, result)
				if err != nil {
					return err
				}
				if changed {
					_, err = tx.ExecContext(ctx, `INSERT INTO code_reviews(execution_id,task_id,base_commit,commit_sha,requirement)
						VALUES($1,$2,$3,$4,$5)`, id, taskID, issue.BaseCommit, result.Commit, issue.Issue.Title+"\n"+issue.Issue.Body)
					if err != nil {
						return err
					}
				}
			}
			if usesCheckpoints {
				var confirmed bool
				err = tx.QueryRowContext(ctx, `SELECT EXISTS(
					SELECT 1 FROM execution_checkpoints turn LEFT JOIN execution_checkpoints next
					ON next.execution_id=turn.execution_id AND next.sequence=turn.sequence+1
					WHERE turn.execution_id=$1 AND turn.session_id=$2
					AND CASE WHEN $3 THEN turn.sequence=0 ELSE next.sequence IS NOT NULL AND next.session_id IS NULL
					AND NOT EXISTS(SELECT 1 FROM checkpoint_comments c WHERE c.execution_id=$1 AND c.sequence=next.sequence) END)`,
					id, result.SessionID, issue.Mode == "plan").Scan(&confirmed)
				if err != nil {
					return err
				}
				if !confirmed {
					return ErrConflict
				}
			}
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
	} else if result.Validation != nil || result.SessionID != "" || result.BaseCommit != "" || result.Plan != nil || result.Harness != "" || result.HarnessVersion != "" {
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
	if sourceClosed {
		status, eventType = "CANCELLED", "source_issue_cancelled"
	} else if !result.succeeded() {
		status, eventType = "FAILED", "execution_failed"
	} else if result.Plan != nil {
		status, eventType = "PLANNING", "planner_completed"
		plan, err := json.Marshal(result.Plan)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO plan_revisions(task_id,revision,plan) VALUES ($1,1,$2)`, taskID, string(plan)); err != nil {
			return err
		}
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
	var abandoned, eligible, sourceClosed bool
	err = tx.QueryRowContext(ctx, `
		SELECT e.task_id, e.abandoned_at IS NOT NULL, e.result IS NULL AND e.lease_until <= now(), t.source_closure_id IS NOT NULL
		FROM executions e JOIN work_items t ON t.id=e.task_id WHERE e.id = $1 FOR UPDATE OF e,t`, id).Scan(&taskID, &abandoned, &eligible, &sourceClosed)
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
	status := "FAILED"
	if sourceClosed {
		status = "CANCELLED"
	}
	return finishTask(ctx, tx, taskID, id, status, "execution_abandoned")
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
