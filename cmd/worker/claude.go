package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"forgeos/internal/store"
)

type claudeCLI struct {
	bin, version string
	streamInput  bool
}

func (c claudeCLI) identity() (string, string) { return "claude", c.version }

var claudeVersion = regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+) \(Claude Code\)$`)
var nativeSessionID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func findClaude(configured string) (claudeCLI, error) {
	if configured == "" {
		return claudeCLI{}, nil
	}
	bin, err := exec.LookPath(configured)
	if err != nil || (runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(bin), ".exe")) {
		return claudeCLI{}, errors.New("FORGEOS_CLAUDE_BIN must identify a native Claude Code executable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := func(arg string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, arg)
		cmd.Env = harnessEnv()
		return cmd.Output()
	}
	help, err := command("--help")
	if err != nil || !hasCLIOptions(help, "--restricted", "--safe-mode", "--strict-mcp-config", "--no-chrome", "--disable-slash-commands",
		"--permission-mode", "--tools", "--verbose", "--output-format", "--print", "--resume") || !bytes.Contains(help, []byte("stream-json")) {
		return claudeCLI{}, errors.New("configured Claude Code must support restricted headless turns, stream-json events and session resume")
	}
	out, err := command("--version")
	version := claudeVersion.FindStringSubmatch(strings.TrimSpace(string(out)))
	if err != nil || len(version) != 2 {
		return claudeCLI{}, errors.New("cannot identify the Claude Code version")
	}
	return claudeCLI{bin: bin, version: version[1], streamInput: hasCLIOptions(help, "--input-format")}, nil
}

func (c claudeCLI) command(ctx context.Context, workspace, prompt, resume string) (*exec.Cmd, error) {
	profile := store.ClaudeImplementationProfile()
	args := []string{"-p", "--restricted", "--safe-mode", "--strict-mcp-config", "--no-chrome", "--disable-slash-commands",
		"--permission-mode", profile.Permission, "--tools", strings.Join(*profile.Tools, ","), "--verbose", "--output-format", "stream-json"}
	config := os.Getenv("CLAUDE_CONFIG_DIR")
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		config = filepath.Join(home, ".claude")
	}
	if !filepath.IsAbs(config) {
		return nil, errors.New("Claude configuration directory must be absolute")
	}
	// Restricted mode ignores user settings by default, including the user's model gateway.
	settings := filepath.Join(config, "settings.json")
	if _, err := os.Stat(settings); err == nil {
		args = append(args, "--settings", settings)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot read Claude user settings")
	}
	if resume != "" {
		if !nativeSessionID.MatchString(resume) {
			return nil, errors.New("invalid Claude session ID")
		}
		args = append(args, "--resume", resume)
	}
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Dir, cmd.Env, cmd.WaitDelay = workspace, harnessEnv(), 3*time.Second
	for _, name := range []string{"CLAUDE_CONFIG_DIR", "CLAUDE_CODE_GIT_BASH_PATH", "CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "ANTHROPIC_MODEL"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	if c.streamInput {
		cmd.Args = append(cmd.Args, "--input-format", "stream-json")
	} else {
		cmd.Stdin = strings.NewReader(prompt)
	}
	return cmd, nil
}

func readClaudeEvents(stream io.Reader, expected string, confirmed func(string) error, activity func(string, []byte) error, question *nativeQuestion, control *claudeControl) (session string, completed bool, err error) {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 16<<10), 4<<20)
	var outcome error
	for scanner.Scan() {
		var event struct {
			Type    string          `json:"type"`
			Subtype string          `json:"subtype"`
			IsError bool            `json:"is_error"`
			Session string          `json:"session_id"`
			Result  string          `json:"result"`
			Errors  json.RawMessage `json:"errors"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return session, completed, errors.New("invalid Claude Code event")
		}
		switch {
		case control != nil && event.Type == "control_response":
			if err := control.response(scanner.Bytes()); err != nil {
				return session, completed, err
			}
		case control != nil && event.Type == "control_request":
			return session, completed, errors.New("unexpected Claude tool or approval request")
		case event.Type == "system" && event.Subtype == "init":
			if session != "" || !nativeSessionID.MatchString(event.Session) || (expected != "" && event.Session != expected) {
				return session, completed, errors.New("unexpected Claude Code session")
			}
			if control != nil {
				control.active.Store(true)
			}
			if err := confirmed(event.Session); err != nil {
				return "", false, err
			}
			session = event.Session
		case activity != nil && session != "" && event.Session == session && !completed && (event.Type == "assistant" || event.Type == "user"):
			if err := activity(event.Type, scanner.Bytes()); err != nil {
				return session, completed, err
			}
		case event.Type == "result":
			if session == "" && !completed && nativeSessionID.MatchString(expected) && event.IsError && event.Subtype == "error_during_execution" {
				var messages []string
				if json.Unmarshal(event.Errors, &messages) == nil && len(messages) == 1 && messages[0] == "No conversation found with session ID: "+expected {
					outcome, completed = errSessionMissing, true
					if control != nil {
						control.end()
					}
					continue
				}
			}
			if completed || session == "" || session != event.Session {
				return session, false, errors.New("mismatched Claude Code result")
			}
			completed = true
			if control != nil {
				control.end()
			}
			if activity != nil {
				if err := activity(event.Type, scanner.Bytes()); err != nil {
					return session, completed, err
				}
			}
			if event.IsError || event.Subtype != "success" {
				outcome = errTurnFailed
			} else if question != nil {
				question.read(event.Result)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return session, completed, err
	}
	return session, completed, outcome
}

func (c claudeCLI) runTurn(ctx context.Context, process processLog, workspace, prompt, resume string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	child := ctx
	if c.streamInput {
		var kill context.CancelFunc
		child, kill = context.WithCancel(context.WithoutCancel(ctx))
		defer kill()
	}
	cmd, err := c.command(child, workspace, prompt, resume)
	if err != nil {
		return "", err
	}
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.New("cannot open Claude result stream")
	}
	var control *claudeControl
	if c.streamInput {
		input, err := cmd.StdinPipe()
		if err != nil {
			stream.Close()
			return "", err
		}
		defer input.Close()
		control = &claudeControl{input: input}
	}
	if err := process.start(cmd, stream); err != nil {
		return "", err
	}
	if control == nil {
		stop := context.AfterFunc(ctx, func() { stream.Close() })
		defer stop()
	} else {
		stop := control.watch(ctx, cmd, stream)
		defer stop()
		err := json.NewEncoder(control.input).Encode(map[string]any{"type": "user", "session_id": resume, "parent_tool_use_id": nil,
			"message": map[string]string{"role": "user", "content": prompt}})
		if err != nil {
			_ = cmd.Cancel()
			stream.Close()
			_, journalErr := process.wait(cmd)
			return "", errors.Join(errors.New("could not write Claude user turn"), journalErr, ctx.Err())
		}
	}
	session, completed, parseErr := readClaudeEvents(stream, resume, process.session, process.claudeActivity, process.question, control)
	if parseErr != nil && !errors.Is(parseErr, errTurnFailed) && !errors.Is(parseErr, errSessionMissing) {
		_ = cmd.Cancel()
		stream.Close()
	}
	if control != nil {
		_ = control.input.Close()
	}
	exit, err := process.wait(cmd)
	switch {
	case err != nil:
		return session, fmt.Errorf("record Claude process exit: %w", err)
	case ctx.Err() != nil:
		return session, ctx.Err()
	case errors.Is(parseErr, errSessionMissing):
		if session == "" && cmd.ProcessState.ExitCode() == 1 {
			return "", errSessionMissing
		}
		return session, errors.New("inconsistent Claude Code resume result")
	case errors.Is(parseErr, errTurnFailed):
		return session, failedTurnExit(exit)
	case parseErr != nil:
		return session, parseErr
	case exit != nil && !completed && session != "":
		return session, fmt.Errorf("%w: %w", errTurnInterrupted, exit)
	case exit != nil || !completed:
		return session, errors.New("Claude Code did not complete the turn")
	}
	return session, nil
}
