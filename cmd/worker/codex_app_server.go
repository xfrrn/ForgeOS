package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"forgeos/internal/store"
)

type codexServerMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type codexServer struct {
	mu           sync.Mutex
	input        *json.Encoder
	scanner      *bufio.Scanner
	pending      []codexServerMessage
	seq          int
	resume       string
	thread, turn string
	interrupt    bool
}

func (s *codexServer) write(message any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input.Encode(message)
}

func checkCodexAppServer(bin string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "app-server", "--help")
	cmd.Env = harnessEnv()
	help, err := cmd.Output()
	if err != nil || !hasCLIOptions(help, "--listen", "--disable", "--config") {
		return errors.New("configured Codex does not support the required App Server options")
	}
	return nil
}

func (s *codexServer) read() (codexServerMessage, error) {
	var m codexServerMessage
	if !s.scanner.Scan() {
		if errors.Is(s.scanner.Err(), bufio.ErrTooLong) {
			return m, errors.New("App Server event exceeds the size limit")
		}
		return m, errTurnInterrupted
	}
	if json.Unmarshal(s.scanner.Bytes(), &m) != nil {
		return m, errors.New("invalid App Server event")
	}
	if m.Method != "" && len(m.ID) != 0 && string(m.ID) != "null" {
		// These roles use never approval and communicate business questions at turn boundaries.
		return m, errors.New("App Server requested approval or tool input outside this role's permissions")
	}
	return m, nil
}

func (s *codexServer) call(method string, params, result any) error {
	s.seq++
	if err := s.write(map[string]any{"id": s.seq, "method": method, "params": params}); err != nil {
		return errors.New("could not write App Server request")
	}
	buffered := 0
	for {
		m, err := s.read()
		if err != nil {
			return err
		}
		if string(m.ID) == strconv.Itoa(s.seq) {
			if m.Error != nil {
				if method == "thread/resume" && m.Error.Code == -32600 && strings.Contains(m.Error.Message, "no rollout found for thread id "+s.resume) {
					return errSessionMissing
				}
				return fmt.Errorf("App Server %s failed (code %d)", method, m.Error.Code)
			}
			if len(m.Result) == 0 || json.Unmarshal(m.Result, result) != nil {
				return errors.New("invalid App Server response")
			}
			return nil
		}
		buffered += len(m.Params)
		if m.Method == "" || len(s.pending) >= 128 || buffered > 4<<20 {
			return errors.New("unexpected App Server response stream")
		}
		s.pending = append(s.pending, m)
	}
}

func (c codexCLI) runAppServer(ctx context.Context, process processLog, workspace, prompt, resume string) (session string, resultErr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if resume != "" && !nativeSessionID.MatchString(resume) {
		return "", errors.New("invalid Codex recovery thread ID")
	}
	args := []string{"app-server", "--listen", "stdio://", "-c", "mcp_servers={}", "-c", `web_search="disabled"`,
		"-c", "notify=[]", "-c", "project_doc_max_bytes=0", "-c", `developer_instructions=""`,
		"-c", "sandbox_workspace_write.network_access=false", "-c", "sandbox_workspace_write.exclude_tmpdir_env_var=true",
		"-c", "sandbox_workspace_write.exclude_slash_tmp=true", "-c", "projects." + strconv.Quote(workspace) + `.trust_level="untrusted"`}
	if runtime.GOOS == "windows" {
		args = append(args, "-c", `windows.sandbox="unelevated"`)
	}
	disabled := c.disabledFeatures
	if disabled == nil {
		disabled = store.CodexDisabledFeatures()
	}
	for _, feature := range disabled {
		args = append(args, "--disable", feature)
	}
	var schema json.RawMessage
	var output string
	for i := 0; i < len(c.options); i += 2 {
		if i+1 == len(c.options) {
			return "", errors.New("invalid App Server output configuration")
		}
		switch c.options[i] {
		case "--output-schema":
			var err error
			schema, err = os.ReadFile(c.options[i+1])
			if err != nil || !json.Valid(schema) {
				return "", errors.New("invalid App Server output schema")
			}
		case "--output-last-message":
			output = c.options[i+1]
		default:
			return "", errors.New("unsupported App Server output option")
		}
	}
	processCtx, kill := context.WithCancel(context.WithoutCancel(ctx))
	defer kill()
	cmd := exec.CommandContext(processCtx, c.bin, args...)
	cmd.Dir, cmd.Env, cmd.WaitDelay = workspace, harnessEnv(), 3*time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		return "", errors.New("could not open App Server input")
	}
	defer input.Close()
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.New("could not open App Server event stream")
	}
	if err = process.start(cmd, stream); err != nil {
		return "", err
	}
	s := codexServer{input: json.NewEncoder(input), scanner: bufio.NewScanner(stream), resume: resume}
	s.scanner.Buffer(make([]byte, 16<<10), 4<<20)
	done, cancelDone := make(chan struct{}), make(chan struct{})
	forceStop := func() { kill(); input.Close(); stream.Close() }
	stop := context.AfterFunc(ctx, func() {
		defer close(cancelDone)
		// Bound both an unresponsive server and a blocked write to its stdin.
		timeout := time.AfterFunc(5*time.Second, forceStop)
		defer timeout.Stop()
		s.mu.Lock()
		active := s.turn != ""
		var err error
		if active {
			s.interrupt = true
			err = s.input.Encode(map[string]any{"id": "forgeos-interrupt", "method": "turn/interrupt", "params": map[string]string{"threadId": s.thread, "turnId": s.turn}})
		}
		s.mu.Unlock()
		if !active || err != nil {
			forceStop()
			return
		}
		<-done
	})
	terminal := false
	defer func() {
		input.Close()
		if resultErr != nil && !terminal {
			stream.Close()
			_ = cmd.Cancel()
		}
		timeout := time.AfterFunc(5*time.Second, func() { _ = cmd.Cancel() })
		exit, journalErr := process.wait(cmd)
		timeout.Stop()
		close(done)
		if !stop() {
			<-cancelDone
		}
		if ctx.Err() != nil {
			resultErr = ctx.Err()
		} else if journalErr != nil {
			resultErr = fmt.Errorf("record App Server exit: %w", journalErr)
		} else if resultErr == nil && exit != nil {
			resultErr = fmt.Errorf("App Server exit: %w", exit)
		}
	}()
	var initialized json.RawMessage
	if err = s.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "forgeos", "version": "1"}}, &initialized); err != nil {
		return "", err
	}
	if err = s.write(map[string]string{"method": "initialized"}); err != nil {
		return "", err
	}
	method := "thread/start"
	params := map[string]any{"cwd": workspace, "sandbox": c.sandbox, "approvalPolicy": store.CodexApproval,
		"baseInstructions": "You are a coding agent executing a ForgeOS work item. Follow its supplied scope, requirements and permissions.", "developerInstructions": ""}
	if resume != "" {
		method = "thread/resume"
		params["threadId"], params["excludeTurns"] = resume, true
	}
	var thread struct {
		Cwd            string              `json:"cwd"`
		Thread         struct{ ID string } `json:"thread"`
		ApprovalPolicy string              `json:"approvalPolicy"`
		Sandbox        struct {
			Type                string   `json:"type"`
			NetworkAccess       bool     `json:"networkAccess"`
			WritableRoots       []string `json:"writableRoots"`
			ExcludeTmpdirEnvVar bool     `json:"excludeTmpdirEnvVar"`
			ExcludeSlashTmp     bool     `json:"excludeSlashTmp"`
		} `json:"sandbox"`
	}
	if err = s.call(method, params, &thread); err != nil {
		return "", err
	}
	expected, expectedErr := os.Stat(workspace)
	actual, actualErr := os.Stat(thread.Cwd)
	if expectedErr != nil || actualErr != nil || !os.SameFile(expected, actual) {
		return "", errors.New("App Server selected a different workspace")
	}
	if !nativeSessionID.MatchString(thread.Thread.ID) || (resume != "" && thread.Thread.ID != resume) || thread.ApprovalPolicy != store.CodexApproval || thread.Sandbox.NetworkAccess {
		return "", errors.New("App Server returned an unexpected thread or permission policy")
	}
	wantSandbox := "readOnly"
	if c.sandbox == store.CodexImplementationSandbox {
		wantSandbox = "workspaceWrite"
		if len(thread.Sandbox.WritableRoots) != 0 || !thread.Sandbox.ExcludeTmpdirEnvVar || !thread.Sandbox.ExcludeSlashTmp {
			return "", errors.New("App Server returned unexpected writable roots")
		}
	}
	if thread.Sandbox.Type != wantSandbox {
		return "", errors.New("App Server returned an unexpected sandbox")
	}
	session = thread.Thread.ID
	if err = process.session(session); err != nil {
		return session, err
	}
	var turn struct{ Turn struct{ ID string } }
	start := map[string]any{"threadId": session, "input": []map[string]string{{"type": "text", "text": prompt}}}
	if schema != nil {
		start["outputSchema"] = schema
	}
	if err = s.call("turn/start", start, &turn); err != nil {
		return session, err
	}
	if turn.Turn.ID == "" {
		return session, errors.New("App Server returned an empty turn ID")
	}
	last := ""
	for {
		var m codexServerMessage
		if len(s.pending) != 0 {
			m, s.pending = s.pending[0], s.pending[1:]
		} else if m, err = s.read(); err != nil {
			return session, err
		}
		if string(m.ID) == `"forgeos-interrupt"` {
			s.mu.Lock()
			requested := s.interrupt
			s.mu.Unlock()
			var response map[string]json.RawMessage
			if !requested || m.Error != nil || json.Unmarshal(m.Result, &response) != nil || response == nil {
				return session, errors.New("invalid App Server interrupt response")
			}
			continue
		}
		switch m.Method {
		case "turn/started", "turn/completed", "item/started", "item/completed":
			var event struct {
				ThreadID string `json:"threadId"`
				TurnID   string `json:"turnId"`
				Turn     struct{ ID, Status string }
				Item     struct{ ID, Type, Status, Text string }
			}
			isTurn := strings.HasPrefix(m.Method, "turn/")
			if json.Unmarshal(m.Params, &event) != nil || event.ThreadID != session ||
				(isTurn && event.Turn.ID != turn.Turn.ID) || (!isTurn && event.TurnID != turn.Turn.ID) {
				return session, errors.New("App Server event does not match the active turn")
			}
			if m.Method == "turn/completed" {
				terminal = true
				s.mu.Lock()
				s.turn = ""
				s.mu.Unlock()
				if ctx.Err() != nil {
					return session, ctx.Err()
				}
				if event.Turn.Status != "completed" {
					return session, errTurnFailed
				}
				if output != "" {
					if !json.Valid([]byte(last)) {
						return session, errors.New("App Server returned no structured final output")
					}
					if err = os.WriteFile(output, []byte(last), 0600); err != nil {
						return session, err
					}
				}
				return session, nil
			}
			if m.Method == "turn/started" {
				s.mu.Lock()
				s.thread, s.turn = session, turn.Turn.ID
				s.mu.Unlock()
			}
			if m.Method == "item/completed" && event.Item.Type == "agentMessage" {
				last = event.Item.Text
				if process.question != nil {
					process.question.read(last)
				}
			}
			if process.tools != nil && strings.HasPrefix(m.Method, "item/") {
				name := map[string]string{"commandExecution": "command_execution", "fileChange": "file_change", "mcpToolCall": "mcp_tool_call", "webSearch": "web_search"}[event.Item.Type]
				phase := "started"
				if m.Method == "item/completed" {
					phase = "completed"
					if event.Item.Status == "failed" || event.Item.Status == "declined" {
						phase = "failed"
					}
				}
				if err = process.recordTool(event.Item.ID, name, phase); err != nil {
					return session, err
				}
			}
		default:
			if m.Method == "" {
				return session, errors.New("unexpected App Server response during the turn")
			}
		}
	}
}
