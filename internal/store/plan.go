package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Plan struct {
	Objective  string      `json:"objective"`
	Milestones []Milestone `json:"milestones"`
}

type Milestone struct {
	Name  string     `json:"name"`
	Items []PlanItem `json:"items"`
}

type PlanItem struct {
	Key          string   `json:"key"`
	Objective    string   `json:"objective"`
	Scope        string   `json:"scope"`
	Acceptance   string   `json:"acceptance_criteria"`
	Deliverables string   `json:"deliverables"`
	Dependencies []string `json:"dependencies,omitempty"`
	ContextRefs  []string `json:"context_refs,omitempty"`
}

type PlanRevision struct {
	Review       *ManagementReview `json:"review,omitempty"`
	MinimumLevel string            `json:"minimum_level,omitempty"`
	ReviewedAt   *time.Time        `json:"reviewed_at,omitempty"`
	TaskID       int64             `json:"task_id"`
	Revision     int               `json:"revision"`
	Plan         Plan              `json:"plan"`
	At           time.Time         `json:"at"`
	GitCommit    string            `json:"git_commit,omitempty"`
}

var planItemKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (p Plan) Validate() error {
	if strings.TrimSpace(p.Objective) == "" || len(p.Milestones) == 0 {
		return errors.New("provide a goal objective and milestones")
	}
	nodes := map[string][]string{}
	for _, m := range p.Milestones {
		if strings.TrimSpace(m.Name) == "" {
			return errors.New("each milestone needs a name")
		}
		for _, item := range m.Items {
			if !planItemKey.MatchString(item.Key) || strings.TrimSpace(item.Objective) == "" || strings.TrimSpace(item.Scope) == "" ||
				strings.TrimSpace(item.Acceptance) == "" || strings.TrimSpace(item.Deliverables) == "" {
				return errors.New("each item needs a key, objective, scope, acceptance criteria and deliverables")
			}
			if _, ok := nodes[item.Key]; ok {
				return fmt.Errorf("duplicate item %s", item.Key)
			}
			nodes[item.Key] = item.Dependencies
		}
	}
	if len(nodes) == 0 {
		return errors.New("detail at least one work item")
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(key string) error {
		deps, found := nodes[key]
		if !found || visiting[key] {
			return errors.New("unknown dependency or cycle in plan")
		}
		if done[key] {
			return nil
		}
		visiting[key] = true
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		delete(visiting, key)
		done[key] = true
		return nil
	}
	for key := range nodes {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) SavePlan(ctx context.Context, taskID int64, previous int, plan Plan) (PlanRevision, error) {
	return s.SavePlanWithPolicy(ctx, taskID, previous, plan, "")
}

func (s *Store) SavePlanWithPolicy(ctx context.Context, taskID int64, previous int, plan Plan, level string) (PlanRevision, error) {
	var rev PlanRevision
	if (level != "" && !decisionLevel(level)) || previous < 0 || previous >= 2147483647 || plan.Validate() != nil {
		return rev, ErrConflict
	}
	doc, err := json.Marshal(plan)
	if err != nil {
		return rev, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return rev, err
	}
	defer tx.Rollback()
	var kind, status, validation string
	err = tx.QueryRowContext(ctx, `SELECT kind,status,COALESCE(spec->>'validation','') FROM work_items WHERE id=$1 AND goal_id IS NULL FOR UPDATE`, taskID).Scan(&kind, &status, &validation)
	if errors.Is(err, sql.ErrNoRows) {
		return rev, ErrNotFound
	}
	if err != nil {
		return rev, err
	}
	if level == "D1" && validation != "docs" {
		return rev, ErrConflict
	}
	rev = PlanRevision{MinimumLevel: level, TaskID: taskID, Revision: previous + 1, Plan: plan}
	var same bool
	err = tx.QueryRowContext(ctx, `SELECT at,plan=$3::jsonb,COALESCE(git_commit,''),COALESCE(minimum_level,'') FROM plan_revisions WHERE task_id=$1 AND revision=$2`,
		taskID, rev.Revision, string(doc)).Scan(&rev.At, &same, &rev.GitCommit, &rev.MinimumLevel)
	if err == nil {
		if !same || (level != "" && rev.MinimumLevel != level) {
			return rev, ErrConflict
		}
		return rev, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return rev, err
	}
	if kind != "codex_issue" || (status != "READY" && status != "FAILED" && status != "WAITING_ACCEPTANCE" && status != "PLANNING") {
		return rev, ErrConflict
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO plan_revisions(task_id,revision,plan,minimum_level)
		SELECT $1,$2,$3,NULLIF($4,'') WHERE $2::integer=(SELECT COALESCE(MAX(revision),0)+1 FROM plan_revisions WHERE task_id=$1)
		RETURNING at`, taskID, rev.Revision, string(doc), level).Scan(&rev.At)
	if errors.Is(err, sql.ErrNoRows) {
		return rev, ErrConflict
	}
	if err != nil {
		return rev, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE work_items SET status='PLANNING' WHERE id=$1`, taskID); err != nil {
		return rev, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO task_events(task_id,type) VALUES ($1,'plan_revised')`, taskID); err != nil {
		return rev, err
	}
	return rev, tx.Commit()
}

func (s *Store) PlanRevision(ctx context.Context, taskID int64, revision int64) (PlanRevision, error) {
	var rev PlanRevision
	var doc []byte
	err := s.db.QueryRowContext(ctx, `SELECT to_jsonb(r) - 'review_lease' FROM plan_revisions r WHERE task_id=$1 AND revision=$2`,
		taskID, revision).Scan(&doc)
	if errors.Is(err, sql.ErrNoRows) {
		return rev, ErrNotFound
	}
	if err != nil {
		return rev, err
	}
	err = json.Unmarshal(doc, &rev)
	return rev, err
}

func (s *Store) RecordPlanArchive(ctx context.Context, taskID, revision int64, sha string) error {
	if !validCommit(sha) {
		return ErrConflict
	}
	var stored string
	err := s.db.QueryRowContext(ctx, `UPDATE plan_revisions SET git_commit=$3
		WHERE task_id=$1 AND revision=$2 AND (git_commit IS NULL OR git_commit=$3) RETURNING git_commit`,
		taskID, revision, sha).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	return err
}
