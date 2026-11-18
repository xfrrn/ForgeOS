package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode"
)

type NativeCommandRequest struct {
	NativeTurn
	Lease   string            `json:"lease_token"`
	Item    string            `json:"item_id"`
	Command string            `json:"command"`
	Cwd     string            `json:"cwd"`
	Patch   []NativePatchFile `json:"patch,omitempty"`
}

func ValidApprovalCommand(command string) bool {
	return strings.TrimSpace(command) != "" && len(command) <= 4096 && strings.IndexFunc(command, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
	}) < 0
}

func ValidCommandDecision(d string) bool { return d == "accept" || d == "decline" || d == "cancel" }

type NativeCommandReceipt struct {
	NativeTurn
	Lease  string `json:"lease_token"`
	Item   string `json:"item_id"`
	Status string `json:"native_status"`
}

func (s *Store) ConfirmNativeCommand(ctx context.Context, execution int64, r NativeCommandReceipt) error {
	if len(r.Lease) != 64 || r.Instance < 1 || !ValidAppTurnID(r.Item) || !ValidAppTurnID(r.Session) || !ValidAppTurnID(r.Turn) ||
		(r.Status != "completed" && r.Status != "declined" && r.Status != "failed" && r.Status != "turn_interrupted") {
		return ErrConflict
	}
	hash := sha256.Sum256([]byte(r.Lease))
	result, err := s.db.ExecContext(ctx, `UPDATE native_command_approvals c SET native_status=$7,confirmed_at=coalesce(confirmed_at,clock_timestamp())
		FROM executions e WHERE e.id=$1 AND e.lease_hash=$2 AND c.execution_id=e.id AND c.instance=$3
		AND c.session_id=$4 AND c.turn_id=$5 AND c.item_id=$6 AND c.decision IS NOT NULL
		AND ($7<>'turn_interrupted' OR (c.patch IS NOT NULL AND c.decision='cancel'))
		AND (c.native_status IS NULL OR c.native_status=$7)`, execution, hash[:], r.Instance, r.Session, r.Turn, r.Item, r.Status)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}

func (s *Store) PollNativeCommand(ctx context.Context, execution int64, r NativeCommandRequest) (string, error) {
	if len(r.Lease) != 64 || r.Instance < 1 || !ValidAppTurnID(r.Item) || !ValidAppTurnID(r.Session) || !ValidAppTurnID(r.Turn) || len(r.Cwd) > 4096 || r.Cwd == "" {
		return "", ErrConflict
	}
	var patch any
	if r.Patch != nil {
		if r.Command != "" || !ValidNativePatch(r.Patch, []string{r.Lease}) {
			return "", ErrConflict
		}
		b, err := json.Marshal(r.Patch)
		if err != nil {
			return "", err
		}
		patch = string(b)
	} else if !ValidApprovalCommand(r.Command) {
		return "", ErrConflict
	}
	hash := sha256.Sum256([]byte(r.Lease))
	var d string
	err := s.db.QueryRowContext(ctx, `WITH active AS (
		SELECT e.id,a.instance,a.session_id,a.turn_id,e.workspace,t.spec->>'base_commit' AS base
		FROM executions e JOIN work_items t ON t.id=e.task_id JOIN open_native_input_turns a ON a.execution_id=e.id
		WHERE e.id=$1 AND e.lease_hash=$2 AND a.instance=$3 AND a.session_id=$4 AND a.turn_id=$5 AND e.native_approvals
		AND e.workspace=$8 AND t.spec->>'base_commit' IS NOT NULL
	), prior AS (
		SELECT c.* FROM active a JOIN native_command_approvals c
		ON (c.execution_id,c.instance,c.session_id,c.turn_id,c.item_id)=(a.id,a.instance,a.session_id,a.turn_id,$6)
	), added AS (
		INSERT INTO native_command_approvals(execution_id,instance,session_id,turn_id,item_id,command,cwd,base_commit,patch)
		SELECT id,instance,session_id,turn_id,$6,$7,workspace,base,$9::jsonb FROM active WHERE NOT EXISTS(SELECT 1 FROM prior)
		ON CONFLICT DO NOTHING RETURNING decision
	) SELECT coalesce(decision,'') FROM added UNION ALL SELECT coalesce(decision,'') FROM prior
	WHERE command=$7 AND cwd=$8 AND patch IS NOT DISTINCT FROM $9::jsonb AND valid_until>clock_timestamp()`, execution, hash[:], r.Instance, r.Session, r.Turn, r.Item, r.Command, r.Cwd, patch).Scan(&d)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrConflict
	}
	return d, err
}

func (s *Store) DecideNativeCommand(ctx context.Context, id int64, decision, digest string) error {
	if !ValidCommandDecision(decision) {
		return ErrConflict
	}
	res, err := s.db.ExecContext(ctx, `UPDATE native_command_approvals c SET decision=$2,decided_at=coalesce(c.decided_at,clock_timestamp())
		WHERE c.id=$1 AND coalesce(encode(sha256(convert_to(c.patch::text,'UTF8')),'hex'),'')=$3
		AND (c.decision=$2 OR (c.decision IS NULL AND EXISTS(SELECT 1 FROM live_command_approvals a WHERE a.id=c.id)))`, id, decision, digest)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return ErrConflict
	}
	return err
}
