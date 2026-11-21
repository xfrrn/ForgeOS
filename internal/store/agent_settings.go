package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"forgeos/internal/agents"
	"github.com/google/uuid"
)

type AgentEntry struct {
	agents.Definition
	Effective     agents.Config `json:"effective"`
	LocalRevision int64         `json:"local_revision"`
	Last          *AgentRun     `json:"last,omitempty"`
}
type AgentRun struct {
	Project int64         `json:"project_id"`
	Node    string        `json:"node"`
	At      time.Time     `json:"at"`
	Harness string        `json:"harness"`
	Model   string        `json:"model"`
	Session string        `json:"session"`
	Outcome string        `json:"outcome"`
	Config  agents.Config `json:"config"`
}

type configReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func agentConfig(ctx context.Context, q configReader, scope int64, d agents.Definition) (AgentEntry, error) {
	entry := AgentEntry{Definition: d, Effective: agents.Config{Settings: agents.Settings{Prompt: d.Prompt, Harness: d.Harnesses[0]}, Key: d.Key, Source: "builtin"}}
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT ON(scope_id) scope_id,revision,settings FROM agent_config_revisions WHERE purpose=$1 AND scope_id IN(0,$2) ORDER BY scope_id,revision DESC`, d.Key, scope)
	if err != nil {
		return entry, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, rev int64
		var raw []byte
		if err = rows.Scan(&id, &rev, &raw); err != nil {
			return entry, err
		}
		if id == scope {
			entry.LocalRevision = rev
		}
		if raw == nil {
			continue
		}
		var settings agents.Settings
		if err = json.Unmarshal(raw, &settings); err != nil {
			return entry, err
		}
		entry.Effective.Settings = settings
		entry.Effective.Revision = rev
		entry.Effective.Source = "global"
		if id != 0 {
			entry.Effective.Source = "project"
		}
	}
	if d.Key == "management-chat" {
		entry.Effective.Harness = "codex"
		entry.Effective.Model = ""
	}
	return entry, rows.Err()
}
func (s *Store) AgentHarness(ctx context.Context, project int64, key string) (string, error) {
	d, ok := agents.Lookup(key)
	if !ok {
		return "", ErrNotFound
	}
	e, err := agentConfig(ctx, s.db, project, d)
	return e.Effective.Harness, err
}
func (s *Store) AgentEntries(ctx context.Context, scope int64) ([]AgentEntry, error) {
	if scope != 0 {
		if _, err := s.Project(ctx, scope); err != nil {
			return nil, err
		}
	}
	out := make([]AgentEntry, 0, len(agents.Definitions))
	for _, d := range agents.Definitions {
		e, err := agentConfig(ctx, s.db, scope, d)
		if err != nil {
			return nil, err
		}
		var run AgentRun
		var raw []byte
		err = s.db.QueryRowContext(ctx, `SELECT project_id,node,observed_at,harness,model,session_id,outcome,config FROM agent_assignments WHERE purpose=$1 AND ($2::bigint=0 OR project_id=$2) AND observed_at IS NOT NULL ORDER BY observed_at DESC,id DESC LIMIT 1`, d.Key, scope).Scan(&run.Project, &run.Node, &run.At, &run.Harness, &run.Model, &run.Session, &run.Outcome, &raw)
		if err == nil {
			if err = json.Unmarshal(raw, &run.Config); err != nil {
				return nil, err
			}
			e.Last = &run
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
func (s *Store) SaveAgentConfig(ctx context.Context, scope int64, d agents.Definition, previous, effective int64, settings *agents.Settings) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if scope != 0 {
		var id int64
		if err = tx.QueryRowContext(ctx, "SELECT id FROM projects WHERE id=$1 FOR KEY SHARE", scope).Scan(&id); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	// Serialize edits for the same purpose, including a project inheriting a global revision.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(132,hashtext($1))", d.Key); err != nil {
		return err
	}
	entry, err := agentConfig(ctx, tx, scope, d)
	if err != nil {
		return err
	}
	if entry.LocalRevision != previous || entry.Effective.Revision != effective {
		return ErrConflict
	}
	var raw any
	if settings != nil {
		if !d.Valid(*settings) {
			return ErrConflict
		}
		next := *settings
		if next.Skills == nil { // Clients predating the selector only edit the other settings.
			next.Skills = entry.Effective.Skills
		}
		if _, err := selectedSkills(ctx, tx, next.Skills); err != nil {
			return err
		}
		b, err := json.Marshal(next)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO agent_config_revisions(scope_id,purpose,settings) VALUES($1,$2,$3)", scope, d.Key, raw); err != nil {
		return err
	}
	return tx.Commit()
}
func freezeAgent(ctx context.Context, tx *sql.Tx, project int64, key, node, requestedModel, requestedHarness string) (*agents.Assignment, error) {
	d, ok := agents.Lookup(key)
	if !ok {
		return nil, ErrConflict
	}
	entry, err := agentConfig(ctx, tx, project, d)
	if err != nil {
		return nil, err
	}
	a := agents.Assignment{Config: entry.Effective, Ticket: uuid.NewString()}
	if requestedModel != "" {
		if key != "management-chat" || !agents.ValidModel(requestedModel) {
			return nil, ErrConflict
		}
		if requestedHarness != "codex" && requestedHarness != "claude" {
			return nil, ErrConflict
		}
		a.Harness, a.Model = requestedHarness, requestedModel
	}
	skills, err := selectedSkills(ctx, tx, a.Skills)
	if err != nil {
		return nil, err
	}
	a.Packages = skills
	if len(skills) > 0 {
		a.Prompt += "\n\nThe owner assigned the following Skills to this Agent. Reading their files is permitted for this task, including in read-only roles. Read each SKILL.md before proceeding; resolve referenced files relative to its directory. Use the ForgeOS Skill reader or the native file reader to open them."
		for _, skill := range skills {
			a.Prompt += "\n- " + skill.Name + ": " + skill.Entry
		}
	}
	raw, err := json.Marshal(a.Config)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(a.Ticket))
	_, err = tx.ExecContext(ctx, "INSERT INTO agent_assignments(project_id,ticket_hash,node,purpose,config) VALUES($1,$2,$3,$4,$5)", project, hash[:], node, key, string(raw))
	return &a, err
}
func (s *Store) RoleAgent(ctx context.Context, project int64, key, node, requestedModel, requestedHarness string, lease []byte) (*agents.Assignment, error) {
	tx, ok := ctx.Value(roleClaimTransactionKey{}).(*sql.Tx)
	if !ok {
		return nil, ErrConflict
	}
	a, err := freezeAgent(ctx, tx, project, key, node, requestedModel, requestedHarness)
	if err != nil {
		return nil, err
	}
	ticket := sha256.Sum256([]byte(a.Ticket))
	_, err = tx.ExecContext(ctx, "UPDATE agent_assignments SET role_lease_hash=$2 WHERE ticket_hash=$1", ticket[:], lease)
	return a, err
}

type AgentObservation struct {
	Ticket  string `json:"ticket"`
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Session string `json:"session"`
	Outcome string `json:"outcome"`
}

func (s *Store) ObserveAgent(ctx context.Context, r AgentObservation) error {
	hash := sha256.Sum256([]byte(r.Ticket))
	result, err := s.db.ExecContext(ctx, `UPDATE agent_assignments SET observed_at=now(),harness=$2,model=$3,session_id=$4,outcome=$5 WHERE ticket_hash=$1 AND assigned_at>now()-interval '30 days'`, hash[:], r.Harness, r.Model, r.Session, r.Outcome)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrNotFound
	}
	return err
}
