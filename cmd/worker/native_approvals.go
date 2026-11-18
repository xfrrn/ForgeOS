package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"

	"forgeos/internal/store"
)

const nativeApprovalMethod = "item/commandExecution/requestApproval"

type commandPolicy struct {
	Project  int64    `json:"project_id"`
	Commands []string `json:"commands"`
	Files    []string `json:"files,omitempty"`
}

func configuredCommandPolicy(raw string, appServer bool) (commandPolicy, error) {
	var p commandPolicy
	if raw == "" {
		return p, nil
	}
	if !appServer || len(raw) > 65536 || json.Unmarshal([]byte(raw), &p) != nil || p.Project < 1 || len(p.Commands)+len(p.Files) == 0 || len(p.Commands) > 32 || len(p.Files) > 32 {
		return commandPolicy{}, errors.New("FORGEOS_CODEX_COMMAND_APPROVALS requires App Server and a project with exact commands or file paths (up to 32 each)")
	}
	for _, path := range p.Files {
		if !store.ValidPatchPath(path) {
			return commandPolicy{}, errors.New("invalid native approval file path")
		}
	}
	for _, command := range p.Commands {
		if !store.ValidApprovalCommand(command) {
			return commandPolicy{}, errors.New("invalid native approval command")
		}
	}
	return p, nil
}

type nativeCommandParams struct {
	ThreadID, TurnID, ItemID, Command, Cwd, Kind, EnvironmentID          string
	ApprovalID, AdditionalPermissions, NetworkApprovalContext, GrantRoot json.RawMessage
}

func (p nativeCommandParams) localScope() bool {
	for _, v := range []json.RawMessage{p.ApprovalID, p.AdditionalPermissions, p.NetworkApprovalContext, p.GrantRoot} {
		if len(v) > 0 && string(v) != "null" {
			return false
		}
	}
	return p.EnvironmentID == "" || p.EnvironmentID == "local"
}

func (p nativeCommandParams) allowed(commands []string, workspace string) bool {
	if !p.localScope() || (p.Kind != "" && p.Kind != "command") ||
		!slices.Contains(commands, p.Command) {
		return false
	}
	a, err := os.Stat(p.Cwd)
	b, other := os.Stat(workspace)
	return err == nil && other == nil && os.SameFile(a, b)
}

func (c *turnInputClient) confirmNativeItem(turn store.NativeTurn, item, status string) error {
	receipt := store.NativeCommandReceipt{NativeTurn: turn, Lease: c.lease, Item: item, Status: status}
	body, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	// Owner input keys are UUIDs. Native tool receipts share their durable outbox.
	key := fmt.Sprintf("command:%d:%s", turn.Instance, item)
	_, err = c.worker.journal.Exec("INSERT INTO turn_input_receipts(execution_id,key,receipt) VALUES(?,?,?) ON CONFLICT DO NOTHING", c.execution, key, body)
	return err
}

func (c *turnInputClient) awaitCommand(ctx context.Context, request store.NativeCommandRequest) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	path := fmt.Sprintf("/internal/executions/%d/native-approvals", c.execution)
	for ctx.Err() == nil {
		var result struct{ Decision string }
		status, err := c.worker.post(ctx, path, request, &result)
		if err == nil && status == http.StatusOK && store.ValidCommandDecision(result.Decision) {
			return result.Decision
		}
		if status != http.StatusOK && status != http.StatusConflict && !temporary(status) {
			break
		}
		wait(ctx)
	}
	return ""
}
