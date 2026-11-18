package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"reflect"
	"regexp"
	"strings"
)

var windowsWorkspaceRoot = regexp.MustCompile(`^([A-Za-z]:/|//[^/]+/[^/]+(/|$))`)

func ValidWorkspaceRoot(root string) bool {
	return !strings.ContainsAny(root, "\x00\r\n") && (strings.HasPrefix(root, "/") || windowsWorkspaceRoot.MatchString(strings.ReplaceAll(root, "\\", "/")))
}

// Workspace paths refer to the Worker filesystem, which can use a different OS.
func workerWorkspace(root, name string) string {
	if windowsWorkspaceRoot.MatchString(strings.ReplaceAll(root, "\\", "/")) {
		return strings.TrimRight(strings.ReplaceAll(root, "/", "\\"), "\\") + "\\" + name
	}
	return path.Join(root, name)
}

func executionLeaseSeconds(kind string) int {
	if kind == "codex_issue" {
		return 25 * 60
	}
	return 2 * 60
}

func (s *Store) RenewExecutionLease(ctx context.Context, id int64, token string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	hash := sha256.Sum256([]byte(token))
	var kind string
	var sourceClosed, ownerStop bool
	err = tx.QueryRowContext(ctx, `SELECT t.kind,t.source_closure_id IS NOT NULL,(e.owner_cancel_requested_at IS NOT NULL OR e.ci_pause_requested_at IS NOT NULL) FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.lease_hash=$2 AND e.result IS NULL AND e.abandoned_at IS NULL AND (t.status='RUNNING' OR t.waiting_execution_id=e.id)
		FOR UPDATE OF e,t`, id, hash[:]).Scan(&kind, &sourceClosed, &ownerStop)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if ownerStop {
		return ErrOwnerStop
	}
	if sourceClosed {
		return ErrSourceClosed
	}
	// Check the database clock after acquiring the row, including time spent waiting for the lock.
	res, err := tx.ExecContext(ctx, `UPDATE executions SET lease_until=clock_timestamp()+$2 * interval '1 second'
		WHERE id=$1 AND lease_until>clock_timestamp()`, id, executionLeaseSeconds(kind))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return tx.Commit()
}

type WorkerCapabilities struct {
	SupportsModelPause       bool  `json:"supports_model_pause,omitempty"`
	SupportsPlannerPause     bool  `json:"supports_planner_pause,omitempty"`
	SupportsReworkAnswers    bool  `json:"supports_rework_answers,omitempty"`
	SupportsGlobalRules      bool  `json:"supports_global_rules,omitempty"`
	SupportsStartup          bool  `json:"supports_startup,omitempty"`
	StartupIntegratedProject int64 `json:"startup_integrated_project,omitempty"`
	SupportsResources        bool  `json:"supports_resources,omitempty"`
	SupportsCodex            bool  `json:"supports_codex"`
	SupportsClaude           bool  `json:"supports_claude"`
	SupportsRework           bool  `json:"supports_rework"`
	SupportsPlanner          bool  `json:"supports_planner"`
	SupportsCheckpoints      bool  `json:"supports_checkpoints"`
	SupportsValidation       bool  `json:"supports_validation"`
	SupportsDeclaredChecks   bool  `json:"supports_declared_checks"`
	SupportsRiskPolicy       bool  `json:"supports_risk_policy"`
}

func (c WorkerCapabilities) intersect(node WorkerCapabilities) WorkerCapabilities {
	integrated := c.StartupIntegratedProject
	if integrated != node.StartupIntegratedProject {
		integrated = 0
	}
	return WorkerCapabilities{
		SupportsModelPause:    c.SupportsModelPause && node.SupportsModelPause,
		SupportsPlannerPause:  c.SupportsPlannerPause && node.SupportsPlannerPause,
		SupportsReworkAnswers: c.SupportsReworkAnswers && node.SupportsReworkAnswers,
		SupportsGlobalRules:   c.SupportsGlobalRules && node.SupportsGlobalRules,
		SupportsStartup:       c.SupportsStartup && node.SupportsStartup, StartupIntegratedProject: integrated,
		SupportsResources:      c.SupportsResources && node.SupportsResources,
		SupportsCodex:          c.SupportsCodex && node.SupportsCodex,
		SupportsClaude:         c.SupportsClaude && node.SupportsClaude,
		SupportsRework:         c.SupportsRework && node.SupportsRework,
		SupportsPlanner:        c.SupportsPlanner && node.SupportsPlanner,
		SupportsCheckpoints:    c.SupportsCheckpoints && node.SupportsCheckpoints,
		SupportsValidation:     c.SupportsValidation && node.SupportsValidation,
		SupportsDeclaredChecks: c.SupportsDeclaredChecks && node.SupportsDeclaredChecks,
		SupportsRiskPolicy:     c.SupportsRiskPolicy && node.SupportsRiskPolicy,
	}
}

type WorkerHeartbeat struct {
	Worker       string                  `json:"worker"`
	OS           string                  `json:"os"`
	Arch         string                  `json:"arch"`
	CPUs         int                     `json:"cpus"`
	Slots        int                     `json:"slots"`
	ModelSlots   int                     `json:"model_slots,omitempty"`
	CISlots      int                     `json:"ci_slots,omitempty"`
	Capabilities WorkerCapabilities      `json:"capabilities"`
	Adapters     map[string]AdapterProbe `json:"-"`
}

type AdapterProbe struct {
	Version    string                  `json:"version"`
	Operations []string                `json:"operations"`
	Profiles   map[string]AgentProfile `json:"profiles,omitempty"`
}

var platformName = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func (h WorkerHeartbeat) Valid() bool {
	if h.Adapters != nil {
		_, codex := h.Adapters["codex"]
		_, claude := h.Adapters["claude"]
		if codex != h.Capabilities.SupportsCodex || claude != h.Capabilities.SupportsClaude {
			return false
		}
		for name, probe := range h.Adapters {
			if probe.Profiles != nil && !reflect.DeepEqual(probe.Profiles, h.Capabilities.Profiles(name)) {
				return false
			}
			if !ValidHarnessVersion(name, probe.Version) || len(probe.Operations) == 0 {
				return false
			}
			seen := make(map[string]bool)
			for _, op := range probe.Operations {
				switch op {
				case "probe", "start", "resume", "events", "interrupt":
				default:
					return false
				}
				if seen[op] {
					return false
				}
				seen[op] = true
			}
		}
	}
	return h.Capabilities.StartupIntegratedProject >= 0 && (h.Capabilities.StartupIntegratedProject == 0 || h.Capabilities.SupportsStartup) &&
		strings.TrimSpace(h.Worker) != "" && len(h.Worker) <= 128 && !strings.ContainsAny(h.Worker, "\x00\r\n") &&
		platformName.MatchString(h.OS) && platformName.MatchString(h.Arch) &&
		((h.ModelSlots == 0 && h.CISlots == 0 && !h.Capabilities.SupportsResources) ||
			(h.Capabilities.SupportsResources && h.ModelSlots >= 1 && h.ModelSlots <= 2147483647 && h.CISlots >= 1 && h.CISlots <= 2147483647)) &&
		h.CPUs >= 1 && h.CPUs <= 2147483647 && h.Slots >= 1 && h.Slots <= 2147483647
}

func (s *Store) RecordHeartbeat(ctx context.Context, h WorkerHeartbeat) error {
	if !h.Valid() {
		return ErrConflict
	}
	caps, err := json.Marshal(struct {
		WorkerCapabilities
		Adapters map[string]AdapterProbe `json:"adapters,omitempty"`
	}{h.Capabilities, h.Adapters})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO worker_nodes(worker,os,arch,cpus,slots,capabilities,model_slots,ci_slots)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7::integer,0),NULLIF($8::integer,0)) ON CONFLICT(worker) DO UPDATE SET
		os=excluded.os,arch=excluded.arch,cpus=excluded.cpus,slots=excluded.slots,
		model_slots=excluded.model_slots,ci_slots=excluded.ci_slots,
		capabilities=excluded.capabilities,last_seen=clock_timestamp()`, h.Worker, h.OS, h.Arch, h.CPUs, h.Slots, caps, h.ModelSlots, h.CISlots)
	return err
}

func (s *Store) WorkerNodes(ctx context.Context) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(n) || to_jsonb(usage) || jsonb_build_object(
		'online',n.last_seen > now() - interval '45 seconds') ORDER BY n.worker),'[]'::jsonb)
		FROM worker_nodes n CROSS JOIN LATERAL (`+workerUsage+`) usage`).Scan(&result)
	return result, err
}
