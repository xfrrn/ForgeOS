package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"

	"forgeos/internal/agents"
	"github.com/lib/pq"
)

type ModelBudget struct {
	Name             string            `json:"name"`
	Harness          string            `json:"harness"`
	Slots            int               `json:"slots"`
	Workers          []string          `json:"workers"`
	ReviewerProjects []int64           `json:"reviewer_projects,omitempty"`
	TestPlanRoles    []ModelBudgetRole `json:"test_plan_roles,omitempty"`
	AgentRoles       []ModelBudgetRole `json:"agent_roles,omitempty"`
}

type ModelBudgetRole struct {
	Project int64  `json:"project_id"`
	Role    string `json:"role"`
}

var budgetName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func (s *Store) SetModelBudgets(ctx context.Context, budgets []ModelBudget) error {
	if budgets == nil {
		return ErrConflict
	}
	names := map[string]bool{}
	members := map[[2]string]bool{}
	reviewers := map[int64]bool{}
	roles := map[ModelBudgetRole]bool{}
	for _, b := range budgets {
		if !budgetName.MatchString(b.Name) || names[b.Name] || (b.Harness != "codex" && b.Harness != "claude") ||
			b.Slots < 1 || b.Slots > 2147483647 || len(b.Workers)+len(b.ReviewerProjects)+len(b.TestPlanRoles)+len(b.AgentRoles) == 0 {
			return ErrConflict
		}
		names[b.Name] = true
		for _, member := range b.TestPlanRoles {
			if member.Project < 1 || (member.Role != "test" && member.Role != "management") || roles[member] {
				return ErrConflict
			}
			roles[member] = true
		}
		for _, member := range b.AgentRoles {
			if member.Project < 1 || !slices.Contains([]string{"curator", "curation_review", "plan_review", "share_review", "repository_analysis", "feedback_classification", "issue_classification", "question_advice"}, member.Role) || roles[member] {
				return ErrConflict
			}
			roles[member] = true
		}
		for _, project := range b.ReviewerProjects {
			if project < 1 || reviewers[project] {
				return ErrConflict
			}
			reviewers[project] = true
		}
		for _, w := range b.Workers {
			key := [2]string{w, b.Harness}
			if strings.TrimSpace(w) == "" || strings.ContainsAny(w, "\x00\r\n") || len(w) > 128 || members[key] {
				return ErrConflict
			}
			members[key] = true
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Keep a claim or reservation from crossing the configuration change.
	if _, err = tx.ExecContext(ctx, `LOCK TABLE model_budgets IN ACCESS EXCLUSIVE MODE; DELETE FROM model_budgets`); err != nil {
		return err
	}
	for _, b := range budgets {
		members, err := json.Marshal(b)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO model_budgets(name,harness,slots,workers,reviewer_projects,test_plan_roles,agent_roles)
			VALUES ($1,$2,$3,COALESCE($4::text[],'{}'),COALESCE($5::bigint[],'{}'),COALESCE($6::jsonb->'test_plan_roles','[]'),COALESCE($6::jsonb->'agent_roles','[]'))`, b.Name, b.Harness, b.Slots, pq.Array(b.Workers), pq.Array(b.ReviewerProjects), string(members)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE project_chat_turns t SET model_budget=b.name FROM model_budgets b
     WHERE t.native_client IS NOT NULL AND t.resource_worker=ANY(b.workers) AND t.requested_harness=b.harness AND t.model_budget IS NULL
     AND t.status='running' AND t.lease_until>clock_timestamp()`); err != nil {
		return err
	}
	// Include work already running at first configuration. Subsequent moves keep its original account until release.
	_, err = tx.ExecContext(ctx, `UPDATE executions e SET model_budget=b.name FROM work_items t,model_budgets b
		WHERE t.id=e.task_id AND t.kind='codex_issue' AND e.worker=ANY(b.workers)
		AND b.harness=COALESCE(NULLIF(t.spec->>'harness',''),'codex') AND e.model_budget IS NULL
		AND (e.resource_kind='model' OR (NOT e.resource_managed AND COALESCE(t.spec->>'verify_commit','')=''))
		AND e.result IS NULL AND e.abandoned_at IS NULL`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE code_reviews r SET model_budget=b.name FROM work_items t,model_budgets b
		WHERE t.id=r.task_id AND t.project_id=ANY(b.reviewer_projects) AND r.model_budget IS NULL AND r.report IS NULL
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(r.lease::text,'UTF8'))),'claude')
		AND r.failed_at IS NULL AND r.lease_until>clock_timestamp()`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE candidate_test_plan_jobs j SET model_budget=b.name
		FROM candidate_test_plans p JOIN work_items t ON t.id=p.task_id,model_budgets b
		WHERE j.plan_id=p.id AND j.model_budget IS NULL AND j.completed_at IS NULL AND j.lease_until>clock_timestamp()
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(j.lease::text,'UTF8'))),'codex')
		AND b.test_plan_roles @> jsonb_build_array(jsonb_build_object('role',j.role,'project_id',t.project_id))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE material_curations c SET model_budget=b.name FROM model_budgets b
		WHERE c.model_budget IS NULL AND c.review IS NULL AND
		CASE WHEN c.extraction IS NULL THEN c.extraction_until ELSE c.review_until END >clock_timestamp()
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to((CASE WHEN c.extraction IS NULL THEN c.extraction_lease ELSE c.review_lease END)::text,'UTF8'))),'codex')
		AND b.agent_roles @> jsonb_build_array(jsonb_build_object('project_id',c.project_id,
			'role',CASE WHEN c.extraction IS NULL THEN 'curator' ELSE 'curation_review' END))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE plan_revisions r SET model_budget=b.name FROM work_items t,model_budgets b
		WHERE r.task_id=t.id AND r.model_budget IS NULL AND r.review IS NULL AND r.review_until>clock_timestamp()
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(r.review_lease::text,'UTF8'))),'codex')
		AND b.agent_roles @> jsonb_build_array(jsonb_build_object('project_id',t.project_id,'role','plan_review'))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE knowledge_shares s SET model_budget=b.name FROM model_budgets b
		WHERE s.model_budget IS NULL AND s.review IS NULL AND s.review_until>clock_timestamp()
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(s.review_lease::text,'UTF8'))),'codex')
		AND b.agent_roles @> jsonb_build_array(jsonb_build_object('project_id',s.project_id,'role','share_review'))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE repository_analyses a SET model_budget=b.name FROM repository_inventories i,model_budgets b
		WHERE a.task_id=i.task_id AND a.model_budget IS NULL AND a.finished_at IS NULL AND a.lease_until>clock_timestamp()
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(a.lease::text,'UTF8'))),'codex')
		AND b.agent_roles @> jsonb_build_array(jsonb_build_object('project_id',i.project_id,'role','repository_analysis'))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE issue_classifications c SET model_budget=b.name FROM model_budgets b
  WHERE c.model_budget IS NULL AND c.suggestion IS NULL AND c.lease_until>clock_timestamp()
  AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(c.lease::text,'UTF8'))),'codex')
		AND b.agent_roles @> jsonb_build_array(jsonb_build_object('project_id',c.project_id,'role','issue_classification'))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE preview_feedback f SET model_budget=b.name FROM static_preview_receipts p,model_budgets b
		WHERE f.preview_id=p.id AND f.model_budget IS NULL AND f.classified_at IS NULL AND f.review_until>clock_timestamp()
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(f.review_lease::text,'UTF8'))),'codex')
		AND b.agent_roles @> jsonb_build_array(jsonb_build_object('project_id',p.project_id,'role','feedback_classification'))`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE question_advice_claims c SET model_budget=b.name
		FROM decision_questions q JOIN work_items t ON t.id=q.task_id,model_budgets b
		WHERE c.question_id=q.id AND c.model_budget IS NULL AND q.management_advice IS NULL AND c.lease_until>clock_timestamp()
		AND b.harness=COALESCE((SELECT config->>'harness' FROM agent_assignments WHERE role_lease_hash=sha256(convert_to(c.lease::text,'UTF8'))),'codex')
		AND b.agent_roles @> jsonb_build_array(jsonb_build_object('project_id',t.project_id,'role','question_advice'))`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// A nil transaction means the worker must wait. The caller records its lease before committing.
func (s *Store) beginManagementClaim(ctx context.Context, project int64, role string, supportsDeadline bool) (*roleClaimTransaction, sql.NullString, error) {
	tx, err := s.beginRoleClaim(ctx)
	if err != nil {
		return nil, sql.NullString{}, err
	}
	pool, available, err := roleModelSlot(ctx, tx.Tx, role, project)
	if err != nil || !available || (pool.Valid && !supportsDeadline) {
		tx.Rollback()
		return nil, pool, err
	}
	return tx, pool, nil
}

type roleHarnessesKey struct{}

func WithRoleHarnesses(ctx context.Context, available []string) context.Context {
	return context.WithValue(ctx, roleHarnessesKey{}, available)
}

var budgetPurposes = map[string]string{"reviewer": "reviewer", "test": "test-agent", "management": "coverage", "curator": "curator", "curation_review": "knowledge-review", "plan_review": "plan-review", "share_review": "share", "repository_analysis": "analysis", "feedback_classification": "feedback", "issue_classification": "classification", "question_advice": "question"}

func roleModelSlot(ctx context.Context, tx *sql.Tx, role string, project int64) (sql.NullString, bool, error) {
	purpose := budgetPurposes[role]
	d, ok := agents.Lookup(purpose)
	if !ok {
		return sql.NullString{}, false, ErrConflict
	}
	// Keep budget selection and the eventual frozen assignment on the same configuration.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock_shared(132,hashtext($1))", purpose); err != nil {
		return sql.NullString{}, false, err
	}
	e, err := agentConfig(ctx, tx, project, d)
	if err != nil {
		return sql.NullString{}, false, err
	}
	if supported, ok := ctx.Value(roleHarnessesKey{}).([]string); ok && !slices.Contains(supported, e.Effective.Harness) {
		return sql.NullString{}, false, nil
	}
	return modelSlot(ctx, tx, e.Effective.Harness, "", role, project)
}

// Execution workers match by node ID. Independent roles match by project and task.
func modelSlot(ctx context.Context, tx *sql.Tx, harness, worker, role string, project int64) (sql.NullString, bool, error) {
	var pool sql.NullString
	var slots, used int
	err := tx.QueryRowContext(ctx, `SELECT name,slots FROM model_budgets WHERE harness=$1
		AND ($2=ANY(workers) OR ($3::text='reviewer' AND $4::bigint=ANY(reviewer_projects))
			OR (test_plan_roles || agent_roles) @> jsonb_build_array(jsonb_build_object('role',$3::text,'project_id',$4::bigint))) FOR UPDATE`, harness, worker, role, project).Scan(&pool, &slots)
	if errors.Is(err, sql.ErrNoRows) {
		return pool, true, nil
	}
	if err != nil {
		return pool, false, err
	}
	// The previous holder can commit while this transaction waits on the account row.
	err = tx.QueryRowContext(ctx, `SELECT reserved FROM model_budget_usage WHERE name=$1`, pool.String).Scan(&used)
	return pool, used < slots, err
}

func (s *Store) ModelBudgets(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(
		to_jsonb(b) - CASE WHEN cardinality(reviewer_projects)=0 THEN 'reviewer_projects' ELSE '' END
		- CASE WHEN test_plan_roles='[]' THEN 'test_plan_roles' ELSE '' END
		- CASE WHEN agent_roles='[]' THEN 'agent_roles' ELSE '' END ORDER BY name
	),'[]') FROM model_budget_usage b`).Scan(&raw)
	return raw, err
}
