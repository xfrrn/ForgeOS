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
}

func configuredCommandPolicy(raw string, appServer bool) (commandPolicy, error) {
	var p commandPolicy
	if raw == "" {
		return p, nil
	}
	if !appServer || len(raw) > 65536 || json.Unmarshal([]byte(raw), &p) != nil || p.Project < 1 || len(p.Commands) == 0 || len(p.Commands) > 32 {
		return commandPolicy{}, errors.New("FORGEOS_CODEX_COMMAND_APPROVALS requires App Server and a project with 1–32 exact commands")
	}
	for _, command := range p.Commands {
		if !store.ValidApprovalCommand(command) {
			return commandPolicy{}, errors.New("invalid native approval command")
		}
	}
	return p, nil
}

type nativeCommandParams struct {
	ThreadID, TurnID, ItemID, Command, Cwd, Kind, EnvironmentID string
	ApprovalID, AdditionalPermissions, NetworkApprovalContext   json.RawMessage
}

func (p nativeCommandParams) allowed(commands []string, workspace string) bool {
	for _, v := range []json.RawMessage{p.ApprovalID, p.AdditionalPermissions, p.NetworkApprovalContext} {
		if len(v) > 0 && string(v) != "null" {
			return false
		}
	}
	if (p.Kind != "" && p.Kind != "command") || (p.EnvironmentID != "" && p.EnvironmentID != "local") ||
		!slices.Contains(commands, p.Command) {
		return false
	}
	a, err := os.Stat(p.Cwd)
	b, other := os.Stat(workspace)
	return err == nil && other == nil && os.SameFile(a, b)
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
