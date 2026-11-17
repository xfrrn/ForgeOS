package store

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/lib/pq"
)

type ModelBudget struct {
	Name    string   `json:"name"`
	Harness string   `json:"harness"`
	Slots   int      `json:"slots"`
	Workers []string `json:"workers"`
}

var budgetName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func (s *Store) SetModelBudgets(ctx context.Context, budgets []ModelBudget) error {
	if budgets == nil {
		return ErrConflict
	}
	names := map[string]bool{}
	members := map[[2]string]bool{}
	for _, b := range budgets {
		if !budgetName.MatchString(b.Name) || names[b.Name] || (b.Harness != "codex" && b.Harness != "claude") ||
			b.Slots < 1 || b.Slots > 2147483647 || len(b.Workers) == 0 {
			return ErrConflict
		}
		names[b.Name] = true
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
		if _, err = tx.ExecContext(ctx, `INSERT INTO model_budgets(name,harness,slots,workers) VALUES ($1,$2,$3,$4)`, b.Name, b.Harness, b.Slots, pq.Array(b.Workers)); err != nil {
			return err
		}
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
	return tx.Commit()
}

func (s *Store) ModelBudgets(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(b) ORDER BY name),'[]') FROM model_budget_usage b`).Scan(&raw)
	return raw, err
}
