package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"forgeos/internal/store"
	"github.com/google/uuid"
)

var errExcludeTurnsUnsupported = errors.New("resuming without turn history requires an experimental capability")

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
	allowCommandRequests bool
	mu                   sync.Mutex
	input                *json.Encoder
	scanner              *bufio.Scanner
	pending              []codexServerMessage
	seq                  int
	resume               string
	thread, turn         string
	interrupt            bool
	steering             map[string]bool
}

func (s *codexServer) write(message any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input.Encode(message)
}

// App Server merges the account's MCP table with overrides, including an empty table.
func (s *codexServer) disableMCP(workspace string) (map[string]map[string]bool, error) {
	var settings struct {
		Config *struct {
			Servers map[string]struct{} `json:"mcp_servers"`
		}
	}
	if err := s.call("config/read", map[string]any{"cwd": workspace, "includeLayers": false}, &settings); err != nil {
		return nil, err
	}
	if settings.Config == nil {
		return nil, errors.New("App Server did not report its effective configuration")
	}
	disabled := make(map[string]map[string]bool, len(settings.Config.Servers))
	for name := range settings.Config.Servers {
		disabled[name] = map[string]bool{"enabled": false}
	}
	return disabled, nil
}

func (s *codexServer) verifyMCPDisabled(thread string, disabled map[string]map[string]bool) error {
	params := map[string]any{"threadId": thread}
	seen := make(map[string]bool)
	for {
		var page struct {
			NextCursor *string
			Data       []struct {
				Name, RuntimeStatus          string
				Tools                        map[string]json.RawMessage
				Resources, ResourceTemplates []json.RawMessage
			}
		}
		if err := s.call("mcpServerStatus/list", params, &page); err != nil {
			return err
		}
		if page.Data == nil {
			return errors.New("App Server did not report its thread MCP inventory")
		}
		for _, entry := range page.Data {
			_, configured := disabled[entry.Name]
			if !configured || (entry.RuntimeStatus != "" && entry.RuntimeStatus != "disabled") ||
				entry.Tools == nil || entry.Resources == nil || entry.ResourceTemplates == nil ||
				len(entry.Tools)+len(entry.Resources)+len(entry.ResourceTemplates) != 0 {
				return errors.New("App Server thread has an unconfirmed MCP permission policy")
			}
		}
		if page.NextCursor == nil {
			return nil
		}
		cursor := *page.NextCursor
		if cursor == "" || seen[cursor] {
			return errors.New("App Server returned an invalid MCP inventory cursor")
		}
		seen[cursor] = true
		params["cursor"] = cursor
	}
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
		if s.allowCommandRequests && (m.Method == nativeApprovalMethod || m.Method == nativePatchMethod) {
			return m, nil
		}
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
		if m.Method == "" && string(m.ID) == strconv.Itoa(s.seq) {
			if m.Error != nil {
				if method == "turn/start" {
					return fmt.Errorf("%w (code %d)", errRejectedWriterStart, m.Error.Code)
				}
				if method == "thread/resume" && m.Error.Code == -32600 && m.Error.Message == "thread/resume.excludeTurns requires experimentalApi capability" {
					return errExcludeTurnsUnsupported
				}
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

func (s *codexServer) initialize() error {
	var initialized json.RawMessage
	if err := s.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "forgeos", "version": "1"}}, &initialized); err != nil {
		return err
	}
	return s.write(map[string]string{"method": "initialized"})
}

func (c codexCLI) appServerArgs(workspace string) []string {
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
	return args
}

func (c codexCLI) runAppServer(ctx context.Context, process processLog, workspace, prompt, resume string) (session string, resultErr error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if resume != "" && !nativeSessionID.MatchString(resume) {
		return "", errors.New("invalid Codex recovery thread ID")
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
	cmd := exec.CommandContext(processCtx, c.bin, c.appServerArgs(workspace)...)
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
	forceStop := func() {
		// EOF can make a server that rejected the interrupt exit successfully.
		_ = cmd.Cancel()
		input.Close()
		stream.Close()
	}
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
		if resultErr != nil && !terminal {
			forceStop()
		} else {
			input.Close()
		}
		grace := 5 * time.Second
		if terminal && resultErr == nil {
			// Stable native servers may take ten seconds to exit after a completed turn and EOF.
			grace = 15 * time.Second
		}
		timeout := time.AfterFunc(grace, func() { _ = cmd.Cancel() })
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
	if c.onAppTurn != nil {
		if err := trackWriterProcess(cmd.Process.Pid); err != nil {
			return "", fmt.Errorf("track writer tools: %w", err)
		}
	}
	if err = s.initialize(); err != nil {
		return "", err
	}
	disabledMCP, err := s.disableMCP(workspace)
	if err != nil {
		return "", err
	}
	policy := store.CodexApproval
	if (len(c.approvalCommands) > 0 || len(c.approvalFiles) > 0) && c.sandbox == store.CodexImplementationSandbox && process.turnInputs != nil {
		policy = "untrusted"
	}
	method := "thread/start"
	params := map[string]any{"cwd": workspace, "sandbox": c.sandbox, "approvalPolicy": policy,
		"config":           map[string]any{"mcp_servers": disabledMCP},
		"baseInstructions": "You are a coding agent executing a ForgeOS work item. Follow its supplied scope, requirements and permissions.", "developerInstructions": ""}
	model := os.Getenv("FORGEOS_CODEX_APP_SERVER_MODEL")
	if model != "" {
		params["model"] = model
	}
	if resume != "" {
		method = "thread/resume"
		params["threadId"], params["excludeTurns"] = resume, true
	}
	var thread struct {
		Model          string
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
	err = s.call(method, params, &thread)
	if errors.Is(err, errExcludeTurnsUnsupported) {
		delete(params, "excludeTurns")
		err = s.call(method, params, &thread)
	}
	if err != nil {
		return "", err
	}
	if model != "" && thread.Model != model {
		return "", errors.New("App Server returned a different model than the Worker requested")
	}
	expected, expectedErr := os.Stat(workspace)
	actual, actualErr := os.Stat(thread.Cwd)
	if expectedErr != nil || actualErr != nil || !os.SameFile(expected, actual) {
		return "", errors.New("App Server selected a different workspace")
	}
	if !nativeSessionID.MatchString(thread.Thread.ID) || (resume != "" && thread.Thread.ID != resume) || thread.ApprovalPolicy != policy || thread.Sandbox.NetworkAccess {
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
	if err = s.verifyMCPDisabled(session, disabledMCP); err != nil {
		return "", err
	}
	if err = process.session(session); err != nil {
		return session, err
	}
	var turn struct{ Turn struct{ ID string } }
	start := map[string]any{"threadId": session, "input": []map[string]string{{"type": "text", "text": prompt}}}
	if schema != nil {
		start["outputSchema"] = schema
	}
	pendingStart := false
	if c.onAppTurn != nil {
		key, err := uuid.NewRandom()
		if err != nil {
			return session, err
		}
		digest := sha256.Sum256([]byte(prompt))
		intent := activeWriterTurn{NativeTurn: store.NativeTurn{Instance: process.id, Session: session}, RequestID: key.String(), PromptHash: digest[:]}
		if err := c.onAppTurn(intent); err != nil {
			return session, err
		}
		start["clientUserMessageId"] = intent.RequestID
		pendingStart = true
		defer func() {
			if pendingStart && ctx.Err() == nil && !errors.Is(resultErr, errRejectedWriterStart) {
				resultErr = fmt.Errorf("%w: %v", errPendingWriterStart, resultErr)
			}
		}()
	}
	s.allowCommandRequests = policy == "untrusted"
	seenApproval := map[string]string{}
	pendingPatches := map[string]nativePatchProposal{}
	var cancelledPatch string
	if err = s.call("turn/start", start, &turn); err != nil {
		return session, err
	}
	if turn.Turn.ID == "" {
		return session, errors.New("App Server returned an empty turn ID")
	}
	last := ""
	var inputPoll store.TurnInputPoll
	var stopInputs context.CancelFunc
	var inputsDone chan struct{}
	defer func() {
		if stopInputs != nil {
			stopInputs()
			input.Close()
			<-inputsDone
		}
	}()
	for {
		var m codexServerMessage
		if len(s.pending) != 0 {
			m, s.pending = s.pending[0], s.pending[1:]
		} else if m, err = s.read(); err != nil {
			return session, err
		}
		if m.Method == nativeApprovalMethod || m.Method == nativePatchMethod {
			var params nativeCommandParams
			if !s.allowCommandRequests || len(m.ID) == 0 || string(m.ID) == "null" || json.Unmarshal(m.Params, &params) != nil || inputPoll.Instance == 0 ||
				params.ThreadID != inputPoll.Session || params.TurnID != inputPoll.Turn || !store.ValidAppTurnID(params.ItemID) {
				return session, errors.New("native command request does not identify the active turn")
			}
			decision := "decline"
			_, seen := seenApproval[params.ItemID]
			if !seen {
				seenApproval[params.ItemID] = ""
				request := store.NativeCommandRequest{NativeTurn: inputPoll.NativeTurn, Lease: inputPoll.Lease, Item: params.ItemID, Cwd: workspace}
				tool := "commandExecution"
				proposal := pendingPatches[params.ItemID]
				if m.Method == nativePatchMethod {
					tool = "fileChange"
					if len(c.approvalFiles) > 0 && params.localScope() && params.Command == "" && params.Kind == "" && params.Cwd == "" {
						request.Patch = proposal.files
					}
				} else if params.allowed(c.approvalCommands, workspace) {
					request.Command = params.Command
				}
				if request.Command != "" || request.Patch != nil {
					decision = process.turnInputs.awaitCommand(ctx, request)
					if decision != "" {
						seenApproval[params.ItemID] = tool
					} else {
						decision = "cancel"
					}
					if request.Patch != nil && decision == "accept" {
						current, err := process.turnInputs.inspectPatch(workspace, proposal.changes, c.approvalFiles)
						if err != nil {
							return session, err
						}
						if !slices.Equal(request.Patch, current) {
							decision = "decline"
						}
					}
					if request.Patch != nil && decision == "cancel" && seenApproval[params.ItemID] != "" {
						cancelledPatch = params.ItemID
					}
				}
			}
			s.mu.Lock()
			if ctx.Err() != nil || s.turn != params.TurnID || s.interrupt {
				decision = "cancel"
			}
			err = s.input.Encode(map[string]any{"id": m.ID, "result": map[string]string{"decision": decision}})
			s.mu.Unlock()
			if err != nil {
				return session, err
			}
			continue
		}
		var responseID string
		if len(m.ID) > 0 && json.Unmarshal(m.ID, &responseID) == nil && strings.HasPrefix(responseID, "forgeos-steer-") {
			key := strings.TrimPrefix(responseID, "forgeos-steer-")
			s.mu.Lock()
			sent := s.steering[key]
			delete(s.steering, key)
			s.mu.Unlock()
			if !sent {
				return session, errors.New("unexpected App Server input response")
			}
			receipt := inputPoll
			receipt.Key, receipt.Outcome = key, "rejected"
			if m.Error == nil {
				var ack struct{ TurnID string }
				if json.Unmarshal(m.Result, &ack) != nil || ack.TurnID != inputPoll.Turn {
					return session, errors.New("App Server input response does not identify the active turn")
				}
				receipt.Outcome = "accepted"
			}
			if err := process.turnInputs.receipt(receipt); err != nil {
				return session, fmt.Errorf("record native input response: %w", err)
			}
			continue
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
		case "item/commandExecution/outputDelta":
			var event struct {
				ThreadID, TurnID, ItemID, Delta string
			}
			if json.Unmarshal(m.Params, &event) != nil || event.ThreadID != session || event.TurnID != turn.Turn.ID {
				return session, errors.New("App Server output does not match the active turn")
			}
			if err = process.commandOutput(event.ItemID, event.Delta); err != nil {
				return session, err
			}
		case "turn/started", "turn/completed", "item/started", "item/completed":
			var event struct {
				ThreadID string `json:"threadId"`
				TurnID   string `json:"turnId"`
				Turn     struct{ ID, Status string }
				Item     struct {
					ID, Type, Status, Text, AggregatedOutput string
					Changes                                  []nativeFileChange
					Result, Error                            json.RawMessage
				}
			}
			isTurn := strings.HasPrefix(m.Method, "turn/")
			if json.Unmarshal(m.Params, &event) != nil || event.ThreadID != session ||
				(isTurn && event.Turn.ID != turn.Turn.ID) || (!isTurn && event.TurnID != turn.Turn.ID) {
				return session, errors.New("App Server event does not match the active turn")
			}
			if m.Method == "turn/completed" {
				if pendingStart {
					return session, errors.New("App Server completed a writer before confirming its start")
				}
				terminal = true
				s.mu.Lock()
				s.turn = ""
				s.mu.Unlock()
				if ctx.Err() != nil {
					return session, ctx.Err()
				}
				if event.Turn.Status == "interrupted" && cancelledPatch != "" {
					if err := process.turnInputs.confirmNativeItem(inputPoll.NativeTurn, cancelledPatch, "turn_interrupted"); err != nil {
						return session, err
					}
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
				if c.onAppCompleted != nil {
					return session, c.onAppCompleted(store.NativeTurn{Instance: process.id, Session: session, Turn: event.Turn.ID})
				}
				return session, nil
			}
			if m.Method == "turn/started" {
				s.mu.Lock()
				s.thread, s.turn = session, turn.Turn.ID
				s.mu.Unlock()
				if err := process.appTurn(event.Turn.ID); err != nil {
					return session, err
				}
				if c.onAppTurn != nil {
					if err := c.onAppTurn(activeWriterTurn{NativeTurn: store.NativeTurn{Instance: process.id, Session: session, Turn: event.Turn.ID}}); err != nil {
						return session, err
					}
					pendingStart = false
				}
				if process.turnInputs != nil && stopInputs == nil {
					inputPoll = store.TurnInputPoll{NativeTurn: store.NativeTurn{Instance: process.id, Session: session, Turn: turn.Turn.ID}, Lease: process.turnInputs.lease}
					s.steering = make(map[string]bool)
					inputCtx, stop := context.WithCancel(ctx)
					stopInputs, inputsDone = stop, make(chan struct{})
					go func() { defer close(inputsDone); process.turnInputs.run(inputCtx, &s, inputPoll) }()
				}
			}
			if m.Method == "item/completed" && event.Item.Type == "agentMessage" {
				last = event.Item.Text
				if err = process.modelMessage(last); err != nil {
					return session, err
				}
				if process.question != nil {
					process.question.read(last)
				}
			}
			if m.Method == "item/started" && event.Item.Type == "fileChange" && s.allowCommandRequests && len(c.approvalFiles) > 0 {
				if _, exists := pendingPatches[event.Item.ID]; exists {
					return session, errors.New("duplicate native patch proposal")
				}
				files, err := process.turnInputs.inspectPatch(workspace, event.Item.Changes, c.approvalFiles)
				if err != nil {
					return session, err
				}
				pendingPatches[event.Item.ID] = nativePatchProposal{changes: event.Item.Changes, files: files}
			}
			if m.Method == "item/completed" {
				delete(pendingPatches, event.Item.ID)
				if tool := seenApproval[event.Item.ID]; tool != "" && event.Item.Type == tool {
					if err := process.turnInputs.confirmNativeItem(inputPoll.NativeTurn, event.Item.ID, event.Item.Status); err != nil {
						return session, err
					}
					if cancelledPatch == event.Item.ID {
						cancelledPatch = ""
					}
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
				output := event.Item.AggregatedOutput
				if name == "file_change" && phase != "started" && process.toolOutput != nil {
					output = fileChangeOutput(event.Item.Changes)
				}
				if name == "mcp_tool_call" && phase != "started" && process.toolOutput != nil {
					output = mcpToolOutput(event.Item.Result, event.Item.Error)
				}
				if err = process.recordTool(event.Item.ID, name, phase, output); err != nil {
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
