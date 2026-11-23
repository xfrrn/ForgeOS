package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"forgeos/internal/agents"
)

type ImplementationStep struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
}

type FeatureRevision struct {
	Start   string         `json:"start"`
	Note    string         `json:"note"`
	Failure string         `json:"failure"`
	Inputs  []FeatureInput `json:"inputs"`
}
type FeatureStep struct {
	Revision *FeatureRevision `json:"revision,omitempty"`
	ImplementationStep
	Parent int64 `json:"parent"`
}
type FeatureInput struct {
	Task      int64  `json:"task"`
	Execution int64  `json:"execution"`
	Name      string `json:"name"`
	Commit    string `json:"commit"`
}

func validateImplementationSteps(steps []ImplementationStep) error {
	if len(steps) == 0 {
		return nil
	}
	if len(steps) < 2 || len(steps) > 8 {
		return ErrConflict
	}
	names := map[string]bool{}
	for i := range steps {
		s := &steps[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Scope = strings.TrimSpace(s.Scope)
		if s.Name == "" || len(s.Name) > 180 || names[s.Name] || s.Scope == "" || len(s.Scope) > 6000 || strings.ContainsRune(s.Scope+s.Name, 0) || (s.Harness != "codex" && s.Harness != "claude") || !agents.ValidModel(s.Model) {
			return ErrConflict
		}
		names[s.Name] = true
	}
	return nil
}
func stepsJSON(steps []ImplementationStep) any {
	if len(steps) == 0 {
		return nil
	}
	raw, _ := json.Marshal(steps)
	return string(raw)
}
func sameImplementationSteps(steps []ImplementationStep, raw json.RawMessage) bool {
	var saved []ImplementationStep
	return json.Unmarshal(raw, &saved) == nil && slices.Equal(steps, saved)
}

func startFeatureBuild(ctx context.Context, tx *sql.Tx, project, task int64, a ChatAction) error {
	var steps []ImplementationStep
	if err := json.Unmarshal(a.Steps, &steps); err != nil {
		return err
	}
	var raw []byte
	err := tx.QueryRowContext(ctx, `UPDATE work_items SET status='BLOCKED',spec=spec||'{"feature_integration":true}' WHERE project_id=$1 AND id=$2 AND status='READY' AND github_issue_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM executions WHERE task_id=$2) RETURNING spec`, project, task).Scan(&raw)
	if err != nil {
		return proposalResult(err)
	}
	var spec IssueSpec
	if err = json.Unmarshal(raw, &spec); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO feature_builds(task_id) VALUES($1)", task); err != nil {
		return err
	}
	body := a.Body
	if a.Kind == "dispatch_issue" {
		body = spec.Issue.Body + "\n\n" + body
	}
	spec.FeatureIntegration = false
	for i, s := range steps {
		spec.FeatureStep = &FeatureStep{ImplementationStep: s, Parent: task}
		spec.Harness = s.Harness
		spec.Issue.Title = s.Name
		spec.Issue.Body = fmt.Sprintf("%s\n\n本次只实现子任务「%s」的范围。其他子任务独立交付。\n%s", body, s.Name, s.Scope)
		data, err := json.Marshal(spec)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO work_items(project_id,kind,status,spec,feature_parent,feature_position) VALUES($1,'codex_issue','READY',$2,$3,$4)`, project, string(data), task, i+1); err != nil {
			return err
		}
	}
	return nil
}

// The project row serializes claim decisions. Only siblings may share that project,
// and all their Git objects must be accessible to the same registered worker.
func featureClaimFilter(alias string) string {
	return strings.ReplaceAll(` AND ((W.feature_parent IS NULL AND NOT coalesce(W.spec ? 'feature_integration',false)) OR
 ($14 AND EXISTS(SELECT 1 FROM feature_builds b JOIN work_items parent ON parent.id=b.task_id
 WHERE b.task_id=coalesce(W.feature_parent,W.id) AND (b.worker IS NULL OR b.worker=$15)
 AND (b.worker IS NOT NULL OR CASE WHEN parent.spec->>'harness'='claude' THEN $16::boolean ELSE $17::boolean END)
 AND (b.worker IS NOT NULL OR NOT EXISTS (SELECT 1 FROM work_items part WHERE part.feature_parent=b.task_id
 AND CASE WHEN part.spec->>'harness'='claude' THEN NOT $16::boolean ELSE NOT $17::boolean END))
 AND (W.feature_parent IS NULL OR parent.status='BLOCKED'))))
 AND NOT EXISTS(SELECT 1 FROM work_items active WHERE active.project_id=W.project_id
 AND (active.status='RUNNING' OR active.waiting_execution_id IS NOT NULL)
 AND (W.feature_parent IS NULL OR W.feature_parent IS DISTINCT FROM active.feature_parent)) `, "W.", alias+".")
}

func loadFeatureInputs(ctx context.Context, tx *sql.Tx, j *Job) error {
	rows, err := tx.QueryContext(ctx, `SELECT s.id,s.spec->'feature_step'->>'name',s.status,coalesce(e.id,0),coalesce(e.result->>'commit',''),coalesce(e.result->>'base_commit','') FROM work_items s LEFT JOIN LATERAL (SELECT id,result FROM executions WHERE task_id=s.id ORDER BY id DESC LIMIT 1)e ON true WHERE s.feature_parent=$1 ORDER BY s.feature_position`, j.TaskID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var input FeatureInput
		var status, base string
		if err = rows.Scan(&input.Task, &input.Name, &status, &input.Execution, &input.Commit, &base); err != nil {
			return err
		}
		if status != "DONE" || !validCommit(input.Commit) || base != j.Spec.BaseCommit {
			return ErrConflict
		}
		j.FeatureInputs = append(j.FeatureInputs, input)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(j.FeatureInputs) < 2 {
		return ErrConflict
	}
	return nil
}

// A revision is bound to the exact failed integration execution. Its frozen
// manifest remains available after a replacement child attempt is scheduled.
func (s *Store) ReviseFeatureStep(ctx context.Context, project, feature, round, task, execution int64, note string) error {
	note = strings.TrimSpace(note)
	if note == "" || len(note) > 6000 || strings.ContainsRune(note, 0) {
		return ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var parent int64
	var failure string
	var manifest []byte
	err = tx.QueryRowContext(ctx, `SELECT p.id,coalesce(left(e.result->>'error',2000),''),e.feature_inputs FROM work_items p JOIN project_chat_actions a ON a.task_id=p.id JOIN project_chat_turns t ON t.id=a.turn_id JOIN feature_proposals r ON r.id=t.proposal_id JOIN executions e ON e.task_id=p.id
 WHERE p.project_id=$1 AND r.feature_id=$2 AND r.id=$3 AND e.id=$4 AND p.status='FAILED' AND p.source_closure_id IS NULL AND e.feature_inputs IS NOT NULL AND e.id=(SELECT max(id) FROM executions WHERE task_id=p.id) FOR UPDATE OF p`, project, feature, round, execution).Scan(&parent, &failure, &manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	var rev FeatureRevision
	rev.Note, rev.Failure = note, failure
	if err = json.Unmarshal(manifest, &rev.Inputs); err != nil {
		return err
	}
	for _, input := range rev.Inputs {
		if input.Task == task {
			rev.Start = input.Commit
		}
	}
	if !validCommit(rev.Start) {
		return ErrConflict
	}
	data, err := json.Marshal(rev)
	if err != nil {
		return err
	}
	var updated int64
	err = tx.QueryRowContext(ctx, `UPDATE work_items SET status='READY',spec=jsonb_set(spec,'{feature_step,revision}',$3::jsonb)
 WHERE id=$1 AND feature_parent=$2 AND status='DONE' RETURNING id`, task, parent, string(data)).Scan(&updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	// Discard any paused CI refresh target: the new attempt needs a fresh assembly.
	_, err = tx.ExecContext(ctx, "UPDATE work_items SET status='BLOCKED',spec=spec-'verify_commit' WHERE id=$1", parent)
	if err != nil {
		return err
	}
	return tx.Commit()
}
