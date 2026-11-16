package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

func (r *Result) manualPlanRequired() bool {
	if r == nil || r.Validation == nil || r.Validation.Run == nil || r.Validation.Run.Decision == nil {
		return false
	}
	for _, m := range r.Validation.Run.Decision.Matches {
		if m.Manual != nil && (string(m.Manual.Required) == "true" || string(m.Manual.Required) == `"conditional"`) {
			return true
		}
	}
	return false
}

type TestCaseResult struct {
	Execution  int64    `json:"execution_id"`
	Revision   int      `json:"revision"`
	Commit     string   `json:"commit"`
	PlanDigest string   `json:"plan_sha256"`
	Case       string   `json:"case_id"`
	Previous   int      `json:"previous_attempt"`
	Result     string   `json:"result"`
	Operations []string `json:"operations"`
	Actual     string   `json:"actual_result"`
	Evidence   []string `json:"evidence"`
}

func (r TestCaseResult) valid() bool {
	if r.Execution < 1 || r.Revision < 1 || r.Previous < 0 || r.Previous >= 2147483647 || !validCommit(r.Commit) || len(r.PlanDigest) != 64 || !validCommit(r.PlanDigest) || r.Case == "" || len(r.Case) > 32 || (r.Result != "passed" && r.Result != "failed" && r.Result != "blocked") {
		return false
	}
	if strings.TrimSpace(r.Actual) == "" || len(r.Actual) > 8192 {
		return false
	}
	for _, items := range [][]string{r.Operations, r.Evidence} {
		if len(items) == 0 || len(items) > 32 {
			return false
		}
		for _, value := range items {
			if strings.TrimSpace(value) == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
				return false
			}
		}
	}
	return !strings.ContainsRune(r.Actual, 0) && utf8.ValidString(r.Actual)
}

func (s *Store) RecordTestCase(ctx context.Context, task, project int64, tester, actor string, r TestCaseResult) (json.RawMessage, error) {
	if !r.valid() || actor == "" || (tester != "runner" && tester != "human") || (tester == "runner" && project < 1) {
		return nil, ErrConflict
	}
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM work_items WHERE id=$1 AND ($2::bigint=0 OR project_id=$2) FOR UPDATE`, task, project).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	var id int64
	var current bool
	err = tx.QueryRowContext(ctx, `SELECT p.id,(p.revision=(SELECT max(revision) FROM candidate_test_plans WHERE execution_id=p.execution_id)
		AND p.execution_id=(SELECT max(id) FROM executions WHERE task_id=p.task_id))
		FROM candidate_test_plan_receipts p WHERE p.task_id=$1 AND p.execution_id=$2 AND p.revision=$3 AND p.state='covered'
		AND p.coverage->>'plan_sha256'=$4 AND p.coverage->>'commit'=$5
		AND EXISTS(SELECT 1 FROM jsonb_array_elements(p.plan->'cases') c WHERE c->>'id'=$6 AND c->>'tester'=$7)`, task, r.Execution, r.Revision, r.PlanDigest, r.Commit, r.Case, tester).Scan(&id, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	var latest int
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT attempt,body=$3::jsonb AND actor=$4 AND tester=$5 FROM candidate_test_case_results WHERE plan_id=$1 AND case_id=$2 ORDER BY attempt DESC LIMIT 1`, id, r.Case, string(body), actor, tester).Scan(&latest, &same)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if latest == r.Previous {
		if !current || status != "WAITING_ACCEPTANCE" {
			return nil, ErrConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO candidate_test_case_results(plan_id,case_id,attempt,tester,actor,body) VALUES($1,$2,$3,$4,$5,$6)`, id, r.Case, latest+1, tester, actor, string(body))
		if err != nil {
			return nil, err
		}
	} else if latest != r.Previous+1 || !same {
		return nil, ErrConflict
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return s.TestPlanReceipt(ctx, task, r.Execution, r.Revision)
}
