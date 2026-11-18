package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"forgeos/internal/acceptance"
	"forgeos/internal/validation"
	"github.com/lib/pq"
)

type TestPlanJob struct {
	CaptureProcess bool              `json:"capture_process,omitempty"`
	ID             int64             `json:"id"`
	Execution      int64             `json:"execution_id"`
	Lease          string            `json:"lease"`
	LeaseUntil     *time.Time        `json:"lease_until,omitempty"`
	Source         acceptance.Source `json:"source"`
	Plan           *acceptance.Plan  `json:"plan,omitempty"`
	CaptureProfile bool              `json:"capture_profile,omitempty"`
}

type TestPlanOutcome struct {
	ID       int64                `json:"id"`
	Lease    string               `json:"lease"`
	Failed   bool                 `json:"failed,omitempty"`
	Plan     *acceptance.Plan     `json:"plan,omitempty"`
	Coverage *acceptance.Coverage `json:"coverage,omitempty"`
	Profile  *AgentProfile        `json:"profile,omitempty"`
}

func (s *Store) RequestTestPlan(ctx context.Context, root string, task, execution int64, commit string, previous int, criteria []string) (json.RawMessage, error) {
	if execution < 1 || !validCommit(commit) || previous < 0 || previous >= 2147483647 || len(criteria) > 127 {
		return nil, ErrConflict
	}
	if criteria == nil {
		criteria = []string{}
	}
	extra, err := json.Marshal(criteria)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var repo string
	var specJSON []byte
	err = tx.QueryRowContext(ctx, `SELECT p.path,t.spec FROM work_items t JOIN projects p ON p.id=t.project_id
		JOIN executions e ON e.task_id=t.id WHERE t.id=$1 AND e.id=$2 AND t.status='WAITING_ACCEPTANCE' AND t.kind='codex_issue'
		AND e.result->>'commit'=$3 AND e.result->'validation'->>'commit'=$3 AND e.result->'validation'->>'exit_code'='0'
		AND e.result->>'base_commit'=t.spec->>'base_commit'
		AND e.id=(SELECT max(id) FROM executions WHERE task_id=t.id) FOR UPDATE OF t`, task, execution, commit).Scan(&repo, &specJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	var current int
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT revision,requested_criteria=$2::jsonb FROM candidate_test_plans WHERE execution_id=$1 ORDER BY revision DESC LIMIT 1`, execution, string(extra)).Scan(&current, &same)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if current != previous && !(current == previous+1 && same) {
		return nil, ErrConflict
	}
	if current == previous {
		var spec IssueSpec
		if err = json.Unmarshal(specJSON, &spec); err != nil {
			return nil, err
		}
		policy := validation.Policy{Target: "pull_request"}
		if spec.Policy != nil {
			policy = *spec.Policy
		} else if validation.ValidLevel(spec.Validation) {
			policy.Minimum = spec.Validation
		} else if spec.Validation == "go-test" {
			policy.Minimum = "V2"
		} else {
			policy.Minimum = "V1"
		}
		requirement := strings.TrimSpace(spec.Issue.Title + "\n" + spec.Issue.Body)
		if requirement != "" {
			criteria = append([]string{requirement}, criteria...)
		}
		src, err := acceptance.ReadSource(ctx, root, repo, spec.BaseCommit, commit, policy, criteria)
		if err != nil {
			return nil, err
		}
		doc, err := json.Marshal(src)
		if err != nil {
			return nil, err
		}
		if len(doc) > 1<<20 {
			return nil, ErrConflict
		}
		_, err = tx.ExecContext(ctx, `WITH p AS (INSERT INTO candidate_test_plans(execution_id,task_id,revision,requested_criteria,source)
			SELECT $1,$2,$3,$4,CASE WHEN rules IS NULL THEN $5::jsonb ELSE
				$5::jsonb || jsonb_build_object('candidate_context',rules,'basis',($5::jsonb->'basis') ||
					jsonb_build_object('candidate_context_sha256',encode(sha256(convert_to(rules::text,'UTF8')),'hex'))) END
			FROM review_rules_for_execution($1) rules RETURNING id) INSERT INTO candidate_test_plan_jobs(plan_id,role) SELECT id,'test' FROM p`, execution, task, previous+1, string(extra), string(doc))
		if err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.TestPlanReceipt(ctx, task, execution, previous+1)
}

func (s *Store) TestPlanReceipt(ctx context.Context, task, execution int64, revision int) (json.RawMessage, error) {
	var doc json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(r)||COALESCE((
		SELECT jsonb_object_agg(CASE role WHEN 'test' THEN 'test_profile' ELSE 'management_profile' END,profile)
		FROM candidate_test_plan_jobs WHERE plan_id=r.id AND profile IS NOT NULL),'{}'::jsonb)||CASE WHEN p.source ? 'candidate_context'
		THEN jsonb_build_object('candidate_context',p.source->'candidate_context') ELSE '{}'::jsonb END||jsonb_build_object(
 'test_processes',COALESCE((SELECT jsonb_agg(mp.process || jsonb_strip_nulls(jsonb_build_object('node',mp.node,'attempt',mp.id,'observed_at',mp.observed_at,'resumed_from',NULLIF(mp.resumed_from,0),'missing_session',NULLIF(mp.missing_session,false))) ORDER BY mp.id) FROM management_processes mp WHERE mp.test_plan_id=r.id AND mp.test_plan_role='test'),'[]'::jsonb),
 'management_processes',COALESCE((SELECT jsonb_agg(mp.process || jsonb_strip_nulls(jsonb_build_object('node',mp.node,'attempt',mp.id,'observed_at',mp.observed_at,'resumed_from',NULLIF(mp.resumed_from,0),'missing_session',NULLIF(mp.missing_session,false))) ORDER BY mp.id) FROM management_processes mp WHERE mp.test_plan_id=r.id AND mp.test_plan_role='management'),'[]'::jsonb),
 'case_results',COALESCE(
		(SELECT jsonb_agg(to_jsonb(c) ORDER BY c.case_id,c.attempt) FROM candidate_test_case_results c WHERE c.plan_id=r.id),'[]'))
		FROM candidate_test_plan_status r JOIN candidate_test_plans p USING(id) WHERE r.task_id=$1 AND r.execution_id=$2 AND ($3=0 OR r.revision=$3) ORDER BY r.revision DESC LIMIT 1`, task, execution, revision).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return doc, err
}

// Both agent roles lease jobs using the same queue. A new revision supersedes pending work.
const currentTestPlan = `t.status='WAITING_ACCEPTANCE' AND p.revision=(SELECT max(revision) FROM candidate_test_plans WHERE execution_id=p.execution_id)
	AND p.execution_id=(SELECT max(id) FROM executions WHERE task_id=t.id)`

func (s *Store) ClaimTestPlan(ctx context.Context, project int64, role string, supportsContext, supportsDeadline bool) (*TestPlanJob, error) {
	if role != "test" && role != "management" {
		return nil, ErrConflict
	}
	tx, err := s.beginRoleClaim(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	pool, available, err := modelSlot(ctx, tx.Tx, "codex", "", role, project)
	if err != nil || !available || (pool.Valid && !supportsDeadline) {
		return nil, err
	}
	var j TestPlanJob
	var src, plan []byte
	err = tx.QueryRowContext(ctx, `WITH picked AS (
		SELECT p.id,p.execution_id,p.source,(SELECT outcome FROM candidate_test_plan_jobs WHERE plan_id=p.id AND role='test') AS plan
		FROM candidate_test_plan_jobs j JOIN candidate_test_plans p ON p.id=j.plan_id JOIN work_items t ON t.id=p.task_id
		WHERE t.project_id=$1 AND j.role=$2 AND ($3 OR NOT (p.source ? 'candidate_context')) AND j.completed_at IS NULL AND (j.lease_until IS NULL OR j.lease_until<=now())
		AND `+currentTestPlan+` ORDER BY p.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED)
		UPDATE candidate_test_plan_jobs j SET lease=gen_random_uuid(),lease_until=clock_timestamp()+interval '6 minutes',model_budget=$4 FROM picked
		WHERE j.plan_id=picked.id AND j.role=$2 RETURNING picked.id,picked.execution_id,j.lease,picked.source,picked.plan,
		CASE WHEN $5 THEN j.lease_until END`, project, role, supportsContext, pool, supportsDeadline).Scan(&j.ID, &j.Execution, &j.Lease, &src, &plan, &j.LeaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(src, &j.Source); err != nil {
		return nil, err
	}
	if plan != nil {
		if err = json.Unmarshal(plan, &j.Plan); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	j.CaptureProfile = true
	return &j, nil
}

func (s *Store) FinishTestPlan(ctx context.Context, project int64, role string, out TestPlanOutcome) error {
	wantPlan := role == "test"
	if !validStructuredProfile(out.Profile) || (out.Failed && out.Profile != nil) || (role != "management" && !wantPlan) || (out.Plan != nil) != (!out.Failed && wantPlan) || (out.Coverage != nil) != (!out.Failed && !wantPlan) {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var srcJSON, planJSON, stored []byte
	var storedProfile []byte
	var dev string
	var active, complete, failed bool
	// Read the latest revision in a fresh snapshot after serializing with revision creation and acceptance.
	var taskID int64
	err = tx.QueryRowContext(ctx, `SELECT t.id FROM work_items t JOIN candidate_test_plans p ON p.task_id=t.id
		WHERE p.id=$1 AND t.project_id=$2 FOR UPDATE OF t`, out.ID, project).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT p.source,(SELECT outcome FROM candidate_test_plan_jobs WHERE plan_id=p.id AND role='test'),
		COALESCE(e.session_id,''),j.outcome,j.profile,j.completed_at IS NOT NULL,j.failed,(j.lease_until>now() AND `+currentTestPlan+`)
		FROM candidate_test_plan_jobs j JOIN candidate_test_plans p ON j.plan_id=p.id JOIN work_items t ON t.id=p.task_id
		LEFT JOIN execution_candidate_identity e ON e.id=candidate_implementation_execution(p.execution_id) WHERE p.id=$1 AND t.id=$2 AND j.role=$3 AND j.lease::text=$4 FOR UPDATE OF j`, out.ID, taskID, role, out.Lease).Scan(&srcJSON, &planJSON, &dev, &stored, &storedProfile, &complete, &failed, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	session := ""
	if out.Plan != nil {
		session = out.Plan.Session
	}
	if out.Coverage != nil {
		session = out.Coverage.Session
	}
	if err = verifyManagementProcess(ctx, tx, ManagementProcessReport{TestPlanID: out.ID, TestPlanRole: role, Lease: out.Lease}, session); err != nil {
		return err
	}
	if !complete && !active {
		// A superseded turn has stopped. Release its slot without publishing its result.
		_, err = tx.ExecContext(ctx, `UPDATE candidate_test_plan_jobs SET lease_until=clock_timestamp()
			WHERE plan_id=$1 AND role=$2 AND model_budget IS NOT NULL AND lease_until>clock_timestamp()`, out.ID, role)
		if err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrConflict
	}
	var body any
	if !out.Failed {
		var src acceptance.Source
		if err = json.Unmarshal(srcJSON, &src); err != nil {
			return err
		}
		var value any
		if wantPlan {
			if (!complete && out.Plan.Session == dev) || out.Plan.Verify(src.Basis, src.Library) != nil {
				return ErrConflict
			}
			value = out.Plan
		} else {
			var plan acceptance.Plan
			if err = json.Unmarshal(planJSON, &plan); err != nil {
				return err
			}
			r, err := plan.Coverage(out.Coverage.Session, out.Coverage.Items)
			if err != nil || (!complete && r.Session == dev) || !reflect.DeepEqual(r, *out.Coverage) {
				return ErrConflict
			}
			value = r
		}
		doc, err := json.Marshal(value)
		if err != nil {
			return err
		}
		body = string(doc)
	}
	var profile any
	if out.Profile != nil {
		doc, err := json.Marshal(out.Profile)
		if err != nil {
			return err
		}
		profile = string(doc)
	}
	if complete {
		var same bool
		err = tx.QueryRowContext(ctx, `SELECT ($1::jsonb IS NOT DISTINCT FROM $2::jsonb) AND ($3::jsonb IS NOT DISTINCT FROM $4::jsonb)`, body, nullableJSON(stored), profile, nullableJSON(storedProfile)).Scan(&same)
		if err != nil {
			return err
		}
		if same && failed == out.Failed {
			return nil
		}
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, `UPDATE candidate_test_plan_jobs SET outcome=$3,failed=$4,completed_at=now(),profile=$5 WHERE plan_id=$1 AND role=$2`, out.ID, role, body, out.Failed, profile)
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if wantPlan && !out.Failed {
		if _, err = tx.ExecContext(ctx, `INSERT INTO candidate_test_plan_jobs(plan_id,role) VALUES($1,'management')`, out.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullableJSON(doc []byte) any {
	if doc == nil {
		return nil
	}
	return string(doc)
}
