package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testAppServerProcess() int {
	const thread = "a7090935-460b-4930-8484-0f2008435a45"
	in, out := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	notify := func(method string, params any) { _ = out.Encode(map[string]any{"method": method, "params": params}) }
	for {
		var req struct {
			ID     int
			Method string
			Params struct {
				Cwd, Sandbox, ApprovalPolicy, ThreadID string
				Input                                  []struct{ Text string }
			}
		}
		if in.Decode(&req) != nil {
			return 0
		}
		var result any = map[string]any{}
		switch req.Method {
		case "initialized":
			continue
		case "initialize":
		case "thread/start", "thread/resume":
			if req.Params.ApprovalPolicy != "never" {
				return 9
			}
			if req.Method == "thread/resume" && req.Params.ThreadID != thread {
				_ = out.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32600, "message": "no rollout found for thread id " + req.Params.ThreadID}})
				continue
			}
			sandbox := "readOnly"
			if req.Params.Sandbox == "workspace-write" {
				sandbox = "workspaceWrite"
			}
			if filepath.Base(req.Params.Cwd) == "unsafe" {
				sandbox = "dangerFullAccess"
			}
			roots := []string{}
			if filepath.Base(req.Params.Cwd) == "roots" {
				roots = []string{filepath.Dir(req.Params.Cwd)}
			}
			policy := "never"
			if filepath.Base(req.Params.Cwd) == "policy" {
				policy = "on-request"
			}
			cwd := req.Params.Cwd
			if filepath.Base(cwd) == "workspace" {
				cwd = filepath.Dir(cwd)
			}
			result = map[string]any{"cwd": cwd, "thread": map[string]string{"id": thread}, "approvalPolicy": policy, "sandbox": map[string]any{
				"type": sandbox, "writableRoots": roots, "networkAccess": filepath.Base(req.Params.Cwd) == "network", "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}}
		case "turn/start":
			prompt := req.Params.Input[0].Text
			notify("turn/started", map[string]any{"threadId": thread, "turn": map[string]string{"id": "turn-1", "status": "inProgress"}})
			_ = out.Encode(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]string{"id": "turn-1"}}})
			if prompt == "missing terminal" {
				return 17
			}
			if prompt == "approval" {
				_ = out.Encode(map[string]any{"id": 900, "method": "item/commandExecution/requestApproval", "params": map[string]string{"threadId": thread}})
				continue
			}
			id, turn := thread, "turn-1"
			switch prompt {
			case "foreign thread":
				id = "another-thread"
			case "missing turn":
				turn = ""
			}
			notify("item/completed", map[string]any{"threadId": id, "turnId": turn, "item": map[string]string{
				"id": "message-1", "type": "agentMessage", "text": `{"forgeos_question":"Which heading?","evidence":"No heading selected."}`}})
			status := "completed"
			if prompt == "failed" {
				status = "failed"
			}
			notify("turn/completed", map[string]any{"threadId": id, "turn": map[string]string{"id": turn, "status": status}})
			continue
		default:
			return 8
		}
		_ = out.Encode(map[string]any{"id": req.ID, "result": result})
	}
}

func TestAppServerTurnIdentityAndPermissions(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"complete", "resume", "missing session", "missing terminal", "failed", "foreign thread", "missing turn", "unsafe", "network", "roots", "policy", "workspace", "approval"} {
		t.Run(scenario, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), scenario)
			if err = os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			schema, output := filepath.Join(root, "schema.json"), filepath.Join(root, "result.json")
			if err = os.WriteFile(schema, []byte(`{"type":"object"}`), 0600); err != nil {
				t.Fatal(err)
			}
			adapter := codexCLI{bin: bin, sandbox: "read-only", appServer: true, options: []string{"--output-schema", schema, "--output-last-message", output}}
			if scenario == "roots" {
				adapter.sandbox = "workspace-write"
			}
			resume := ""
			if scenario == "resume" {
				resume = "a7090935-460b-4930-8484-0f2008435a45"
			} else if scenario == "missing session" {
				resume = "046dddd8-2527-45e7-a67f-701b406412aa"
			}
			var question nativeQuestion
			session, err := adapter.runTurn(context.Background(), processLog{question: &question}, root, scenario, resume)
			if scenario == "complete" || scenario == "resume" {
				raw, readErr := os.ReadFile(output)
				if err != nil || readErr != nil || !nativeSessionID.MatchString(session) || !strings.Contains(string(raw), "Which heading?") {
					t.Fatal(session, err, readErr)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted invalid native result")
			}
			if (scenario == "missing terminal") != errors.Is(err, errTurnInterrupted) ||
				(scenario == "failed") != errors.Is(err, errTurnFailed) || (scenario == "missing session") != errors.Is(err, errSessionMissing) {
				t.Fatal("incorrect recovery classification", err)
			}
			if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("invalid turn published output", err)
			}
		})
	}
}
