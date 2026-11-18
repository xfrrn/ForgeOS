package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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
				ExcludeTurns                                   bool
				Model                                          string
				Cwd, Sandbox, ApprovalPolicy, ThreadID, TurnID string
				ExpectedTurnID, ClientUserMessageID            string
				Input                                          []struct{ Text string }
			}
		}
		if in.Decode(&req) != nil {
			if scenario == "slow exit" {
				time.Sleep(6 * time.Second)
			}
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
			if req.Method == "thread/resume" && filepath.Base(cwd) == "stable resume" && req.Params.ExcludeTurns {
				_ = out.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32600, "message": "thread/resume.excludeTurns requires experimentalApi capability"}})
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
			model := req.Params.Model
			if filepath.Base(cwd) == "other model" {
				model = "unexpected"
			}
			result = map[string]any{"cwd": cwd, "model": model, "thread": map[string]string{"id": thread}, "approvalPolicy": policy, "sandbox": map[string]any{
				"type": sandbox, "writableRoots": roots, "networkAccess": filepath.Base(req.Params.Cwd) == "network", "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}}
		case "turn/start":
			prompt := req.Params.Input[0].Text
			switch prompt {
			case "lost start response":
				return 0
			case "rejected start":
				if !nativeSessionID.MatchString(req.Params.ClientUserMessageID) {
					return 10
				}
				_ = out.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32600}})
				continue
			}
			scenario = prompt
			if prompt != "before turn" && prompt != "completed without start" {
				notify("turn/started", map[string]any{"threadId": thread, "turn": map[string]string{"id": "turn-1", "status": "inProgress"}})
			}
			_ = out.Encode(map[string]any{"id": req.ID, "result": map[string]any{"turn": map[string]string{"id": "turn-1"}}})
			if strings.HasPrefix(prompt, "steer ") {
				continue
			}
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

			if strings.HasPrefix(prompt, "SIMULATED_FILE_CHANGE") {
				item := map[string]any{"id": "change-1", "type": "fileChange", "status": "inProgress", "changes": testFileChangeFixture(true), "arguments": "private-input"}
				notify("item/started", map[string]any{"threadId": id, "turnId": turn, "item": item})
				item["status"] = "completed"
				if strings.Contains(prompt, "FAILED") {
					item["status"] = "failed"
				}
				if strings.Contains(prompt, "DECLINED") {
					item["status"] = "declined"
				}
				notify("item/completed", map[string]any{"threadId": id, "turnId": turn, "item": item})
			}
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
		case "turn/steer":
			if req.Params.ThreadID != thread || req.Params.ExpectedTurnID != "turn-1" || req.Params.ClientUserMessageID != "test-owner-input" || len(req.Params.Input) != 1 || req.Params.Input[0].Text != "Use the fixture heading." {
				return 12
			}
			if scenario == "steer rejected" {
				_ = out.Encode(map[string]any{"id": req.ID, "error": map[string]any{"code": -32600, "message": "turn ended"}})
			} else {
				_ = out.Encode(map[string]any{"id": req.ID, "result": map[string]string{"turnId": "another-turn"}})
			}
			notify("turn/completed", map[string]any{"threadId": thread, "turn": map[string]string{"id": "turn-1", "status": "completed"}})
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

func TestAppServerSteerResponseBinding(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"steer rejected", "steer wrong turn"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				var poll store.TurnInputPoll
				if r.URL.Path != "/internal/executions/17/turn-inputs" || json.NewDecoder(r.Body).Decode(&poll) != nil || len(poll.Claim) != 32 || poll.Turn != "turn-1" {
					t.Error("invalid native input poll")
					rw.WriteHeader(400)
					return
				}
				_ = json.NewEncoder(rw).Encode(store.OwnerInstruction{Key: "test-owner-input", Body: "Use the fixture heading."})
			}))
			defer srv.Close()
			root := t.TempDir()
			t.Setenv("FORGEOS_WORKER_STATE_DIR", filepath.Join(t.TempDir(), "state"))
			w := worker{repoRoot: root, workspaceRoot: root, token: "node", codexAppServer: true, client: srv.Client(), api: srv.URL}
			w.journal, err = w.openJournal(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer w.journal.Close()
			job := store.Job{ExecutionID: 17, RecordAppTurn: true, SupportsTurnInput: true}
			adapter := codexCLI{bin: bin, sandbox: "read-only", appServer: true}
			_, turnErr := adapter.runTurn(ctx, w.processLog(ctx, job, "codex", "1.2.3"), root, scenario, "")
			var count int
			if err := w.journal.QueryRow(`SELECT count(*) FROM turn_input_receipts WHERE json_extract(receipt,'$.outcome')='rejected'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			var pending int
			if err := w.journal.QueryRow("SELECT count(*) FROM pending_native_inputs").Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if scenario == "steer rejected" {
				if turnErr != nil || count != 1 || pending != 0 {
					t.Fatal("rejection receipt", turnErr, count, pending)
				}
			} else if turnErr == nil || !strings.Contains(turnErr.Error(), "active turn") || count != 0 || pending != 1 {
				t.Fatal("wrong turn confirmed or lost its unresolved input", turnErr, count, pending)
			}
		})
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
			job := store.Job{RecordToolActivity: true, RecordToolOutput: true, RecordToolStream: true, RecordAppTurn: true}
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
						if !p.Valid() || p.Turn != "turn-1" || p.Exited != nil || !p.ToolOutput.Partial || p.ToolOutput.Text != "Early output\n[redacted]\n" {
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
				if runErr != nil || err != nil || !p.Valid() || p.Turn != "turn-1" || p.ToolOutput == nil || p.ToolOutput.Partial || p.Exited == nil {
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

func TestAppServerPendingStartFailure(t *testing.T) {
	if !supportsWriterProcessTree {
		t.Skip("requires a supported Worker platform")
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		prompt string
		want   error
	}{
		{"lost start response", errPendingWriterStart}, {"rejected start", errRejectedWriterStart},
		{"completed without start", errPendingWriterStart},
	} {
		t.Run(tc.prompt, func(t *testing.T) {
			var saved []activeWriterTurn
			c := codexCLI{bin: bin, appServer: true, sandbox: "read-only", onAppTurn: func(a activeWriterTurn) error {
				saved = append(saved, a)
				return nil
			}}
			requests := make(chan bool, 2)
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				var req store.ResourceRequest
				if json.NewDecoder(r.Body).Decode(&req) != nil || req.Step != 1 || req.Kind != "model" {
					t.Error("invalid resource request")
				}
				requests <- req.Release
				rw.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			b := executionResources{worker: &worker{api: server.URL, client: server.Client()}, job: store.Job{ManageResources: true}}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			session, err := withResource(ctx, &b, "model", func() (string, error) {
				return c.runTurn(ctx, processLog{}, t.TempDir(), tc.prompt, "")
			})
			close(requests)
			var got []bool
			for release := range requests {
				got = append(got, release)
			}
			want := []bool{false}
			if tc.want == errRejectedWriterStart {
				want = append(want, true)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("model reservation release flags %v, want %v", got, want)
			}
			digest := sha256.Sum256([]byte(tc.prompt))
			if !errors.Is(err, tc.want) || errors.Is(err, errTurnInterrupted) || len(saved) != 1 || !saved[0].validReceipt() || saved[0].Session != session || !bytes.Equal(saved[0].PromptHash, digest[:]) {
				t.Fatal(session, saved, err)
			}
		})
	}
}

func TestAppServerTurnIdentityAndPermissions(t *testing.T) {
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"complete", "slow exit", "resume", "stable resume", "other model", "missing session", "missing terminal", "failed", "foreign thread", "missing turn", "unsafe", "network", "roots", "policy", "workspace", "approval"} {
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
			if scenario == "complete" || strings.HasSuffix(scenario, "resume") || scenario == "other model" {
				t.Setenv("FORGEOS_CODEX_APP_SERVER_MODEL", "test-model")
			}
			if strings.HasSuffix(scenario, "resume") {
				resume = "a7090935-460b-4930-8484-0f2008435a45"
			} else if scenario == "missing session" {
				resume = "046dddd8-2527-45e7-a67f-701b406412aa"
			}
			var question nativeQuestion
			session, err := adapter.runTurn(context.Background(), processLog{question: &question}, root, scenario, resume)
			if scenario == "complete" || scenario == "slow exit" || strings.HasSuffix(scenario, "resume") {
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
