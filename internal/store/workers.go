package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

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
	err = tx.QueryRowContext(ctx, `SELECT t.kind FROM executions e JOIN work_items t ON t.id=e.task_id
		WHERE e.id=$1 AND e.lease_hash=$2 AND e.result IS NULL AND e.abandoned_at IS NULL AND t.status='RUNNING'
		FOR UPDATE OF e`, id, hash[:]).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
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
	SupportsCodex          bool `json:"supports_codex"`
	SupportsClaude         bool `json:"supports_claude"`
	SupportsRework         bool `json:"supports_rework"`
	SupportsPlanner        bool `json:"supports_planner"`
	SupportsCheckpoints    bool `json:"supports_checkpoints"`
	SupportsValidation     bool `json:"supports_validation"`
	SupportsDeclaredChecks bool `json:"supports_declared_checks"`
	SupportsRiskPolicy     bool `json:"supports_risk_policy"`
}

func (c WorkerCapabilities) intersect(node WorkerCapabilities) WorkerCapabilities {
	return WorkerCapabilities{
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
	Worker       string             `json:"worker"`
	OS           string             `json:"os"`
	Arch         string             `json:"arch"`
	CPUs         int                `json:"cpus"`
	Slots        int                `json:"slots"`
	Capabilities WorkerCapabilities `json:"capabilities"`
}

var platformName = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func (h WorkerHeartbeat) Valid() bool {
	return strings.TrimSpace(h.Worker) != "" && len(h.Worker) <= 128 && !strings.ContainsAny(h.Worker, "\x00\r\n") &&
		platformName.MatchString(h.OS) && platformName.MatchString(h.Arch) &&
		h.CPUs >= 1 && h.CPUs <= 2147483647 && h.Slots >= 1 && h.Slots <= 2147483647
}

func (s *Store) RecordHeartbeat(ctx context.Context, h WorkerHeartbeat) error {
	if !h.Valid() {
		return ErrConflict
	}
	caps, err := json.Marshal(h.Capabilities)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO worker_nodes(worker,os,arch,cpus,slots,capabilities)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(worker) DO UPDATE SET
		os=excluded.os,arch=excluded.arch,cpus=excluded.cpus,slots=excluded.slots,
		capabilities=excluded.capabilities,last_seen=clock_timestamp()`, h.Worker, h.OS, h.Arch, h.CPUs, h.Slots, caps)
	return err
}

func (s *Store) WorkerNodes(ctx context.Context) (json.RawMessage, error) {
	var result json.RawMessage
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(n) || jsonb_build_object(
		'online',n.last_seen > now() - interval '45 seconds',
		'reserved',(SELECT count(*) FROM executions e WHERE e.worker=n.worker AND e.result IS NULL AND e.abandoned_at IS NULL)
		) ORDER BY n.worker),'[]'::jsonb) FROM worker_nodes n`).Scan(&result)
	return result, err
}
