package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/store"
)

func testAppServerProcess() int {
	const thread = "a7090935-460b-4930-8484-0f2008435a45"
	in, out := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	notify := func(method string, params any) { _ = out.Encode(map[string]any{"method": method, "params": params}) }
	var cwd, scenario string
	for {
		var req struct {
			ID     json.RawMessage
			Method string
			Params struct {
				Cwd, Sandbox, ApprovalPolicy, ThreadID, TurnID string
				Input                                          []struct{ Text string }
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
			cwd = req.Params.Cwd
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
			scenario = prompt
			if prompt != "before turn" {
				notify("turn/started", map[string]any{"threadId": thread, "turn": map[string]string{"id": "turn-1", "status": "inProgress"}})
			}
			_ = out.Encode(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]string{"id": "turn-1"}}})
			if strings.HasPrefix(prompt, "stream ") {
				notify("item/started", map[string]any{"threadId": thread, "turnId": "turn-1", "item": map[string]string{"id": "tool-1", "type": "commandExecution"}})
				id, turnID := thread, "turn-1"
				if prompt == "stream foreign thread" {
					id = "other"
				}
				if prompt == "stream foreign turn" {
					turnID = "other"
				}
				for _, delta := range []string{"Early output\npass", "word=private-value\n"} {
					notify("item/commandExecution/outputDelta", map[string]string{"threadId": id, "turnId": turnID, "itemId": "tool-1", "delta": delta})
				}
				if prompt == "stream live" {
					deadline := time.Now().Add(10 * time.Second)
					for {
						if _, err := os.Stat(filepath.Join(cwd, "release")); err == nil {
							break
						}
						if time.Now().After(deadline) {
							return 18
						}
						time.Sleep(10 * time.Millisecond)
					}
				}
				notify("item/completed", map[string]any{"threadId": thread, "turnId": "turn-1", "item": map[string]string{"id": "tool-1", "type": "commandExecution", "status": "completed", "aggregatedOutput": "Early output\npassword=private-value\n"}})
			}
			if strings.Contains(prompt, "interrupt") || prompt == "before turn" {
				notify("item/started", map[string]any{"threadId": thread, "turnId": "turn-1", "item": map[string]string{"id": "tool-1", "type": "commandExecution"}})
				continue
			}
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
			message := `{"forgeos_question":"Which heading?","evidence":"No heading selected."}`
			if prompt == "SIMULATED_PUBLIC_OUTPUT" {
				notify("item/started", map[string]any{"threadId": id, "turnId": turn, "item": map[string]string{"id": "tool-1", "type": "commandExecution", "command": "private-input"}})
				notify("item/completed", map[string]any{"threadId": id, "turnId": turn, "item": map[string]string{"id": "tool-1", "type": "commandExecution", "status": "completed", "aggregatedOutput": "Tool output sample.\npassword=private-value"}})
				notify("item/completed", map[string]any{"threadId": id, "turnId": turn, "item": map[string]string{"id": "thought", "type": "reasoning", "text": "private-thought"}})
				message = "Reading the guide.\npassword=private-value"
			}
			notify("item/completed", map[string]any{"threadId": id, "turnId": turn, "item": map[string]string{
				"id": "message-1", "type": "agentMessage", "text": message}})
			status := "completed"
			if prompt == "failed" {
				status = "failed"
			}
			notify("turn/completed", map[string]any{"threadId": id, "turn": map[string]string{"id": turn, "status": status}})
			continue
		case "turn/interrupt":
			if req.Params.ThreadID != thread || req.Params.TurnID != "turn-1" {
				return 10
			}
			if err := os.WriteFile(filepath.Join(cwd, "interrupted"), []byte(scenario), 0600); err != nil {
				return 11
			}
			if scenario == "ignore interrupt" {
				continue
			}
			if scenario == "reject interrupt" {
				_ = out.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32600, "message": "cannot interrupt"}})
				continue
			}
			_ = out.Encode(map[string]any{"id": req.ID, "result": result})
			status := "interrupted"
			if scenario == "complete during interrupt" {
				status = "completed"
			}
			notify("turn/completed", map[string]any{"threadId": thread, "turn": map[string]string{"id": "turn-1", "status": status}})
			continue
		default:
			return 8
		}
		_ = out.Encode(map[string]any{"id": req.ID, "result": result})
	}
}

func TestAppServerCommandStreamBeforeCompletion(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"stream live", "stream foreign thread", "stream foreign turn"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			w := worker{repoRoot: root, workspaceRoot: root, token: "node"}
			w.journal, err = w.openJournal(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			adapter := codexCLI{bin: bin, sandbox: "read-only", appServer: true}
			job := store.Job{RecordToolActivity: true, RecordToolOutput: true, RecordToolStream: true}
			end := make(chan error, 1)
			go func() {
				_, err := adapter.runTurn(ctx, w.processLog(ctx, job, "codex", "1.2.3"), root, scenario, "")
				end <- err
			}()
			if scenario == "stream live" {
				deadline := time.Now().Add(8 * time.Second)
				for {
					p, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess + " ORDER BY instance DESC LIMIT 1"))
					if err == nil && p.ToolOutput != nil && strings.Contains(p.ToolOutput.Text, "[redacted]") {
						if !p.Valid() || p.Exited != nil || !p.ToolOutput.Partial || p.ToolOutput.Text != "Early output\n[redacted]\n" {
							t.Fatal(p)
						}
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("no output before command completed", err)
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err := os.WriteFile(filepath.Join(root, "release"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			runErr := <-end
			if scenario == "stream live" {
				p, err := scanHarnessProcess(w.journal.QueryRow(readHarnessProcess + " ORDER BY instance DESC LIMIT 1"))
				if runErr != nil || err != nil || !p.Valid() || p.ToolOutput == nil || p.ToolOutput.Partial || p.Exited == nil {
					t.Fatal(p, runErr, err)
				}
			} else if runErr == nil || !strings.Contains(runErr.Error(), "active turn") {
				t.Fatal(runErr)
			}
		})
	}
}

func TestAppServerInterruptAndFallback(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"interrupt", "complete during interrupt", "ignore interrupt", "reject interrupt", "before turn"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			w := worker{repoRoot: root, workspaceRoot: root}
			w.journal, err = w.openJournal(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			output := filepath.Join(root, "result.json")
			c := codexCLI{bin: bin, sandbox: "read-only", appServer: true, options: []string{"--output-last-message", output}}
			type outcome struct {
				session string
				err     error
			}
			done := make(chan outcome, 1)
			go func() {
				s, e := c.runTurn(ctx, w.processLog(ctx, store.Job{RecordToolActivity: true}, "codex", "1.2.3"), root, scenario, "")
				done <- outcome{s, e}
			}()
			for {
				var events int
				if e := w.journal.QueryRow("SELECT COUNT(*) FROM harness_tool_events").Scan(&events); e != nil {
					t.Fatal(e)
				}
				if events > 0 {
					break
				}
				if ctx.Err() != nil {
					t.Fatal("never reached active tool", ctx.Err())
				}
				time.Sleep(10 * time.Millisecond)
			}
			start := time.Now()
			cancel()
			got := <-done
			if !errors.Is(got.err, context.Canceled) || got.session != "a7090935-460b-4930-8484-0f2008435a45" || time.Since(start) > 9*time.Second {
				t.Fatal("did not stop the owned turn promptly", got, time.Since(start))
			}
			marker, e := os.ReadFile(filepath.Join(root, "interrupted"))
			if scenario == "before turn" {
				if !errors.Is(e, os.ErrNotExist) {
					t.Fatal("sent interrupt before turn/started", e)
				}
			} else if e != nil || string(marker) != scenario {
				t.Fatal("native interrupt was not sent", e)
			}
			var exit int
			if e := w.journal.QueryRow("SELECT exit_code FROM harness_processes").Scan(&exit); e != nil {
				t.Fatal(e)
			}
			graceful := scenario == "interrupt" || scenario == "complete during interrupt"
			if (exit == 0) != graceful {
				t.Fatal("incorrect process shutdown", exit)
			}
			if _, e := os.Stat(output); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("canceled turn published output", e)
			}
		})
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
