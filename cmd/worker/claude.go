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
	return claudeCLI{bin, version[1]}, nil
}

func (c claudeCLI) command(ctx context.Context, workspace, prompt, resume string) (*exec.Cmd, error) {
	profile := store.ClaudeImplementationProfile()
	args := []string{"-p", "--restricted", "--safe-mode", "--strict-mcp-config", "--no-chrome", "--disable-slash-commands",
		"--permission-mode", profile.Permission, "--tools", strings.Join(profile.Tools, ","), "--verbose", "--output-format", "stream-json"}
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
	cmd.Stdin = strings.NewReader(prompt)
	return cmd, nil
}

func readClaudeEvents(stream io.Reader, expected string, confirmed func(string) error, activity func(string, []byte) error, question *nativeQuestion) (session string, completed bool, err error) {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 16<<10), 4<<20)
	var outcome error
	for scanner.Scan() {
		var event struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			IsError bool   `json:"is_error"`
			Session string `json:"session_id"`
			Result  string `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return session, completed, errors.New("invalid Claude Code event")
		}
		switch {
		case event.Type == "system" && event.Subtype == "init":
			if session != "" || !nativeSessionID.MatchString(event.Session) || (expected != "" && event.Session != expected) {
				return session, completed, errors.New("unexpected Claude Code session")
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
			if completed || session == "" || session != event.Session {
				return session, false, errors.New("mismatched Claude Code result")
			}
			completed = true
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
	cmd, err := c.command(ctx, workspace, prompt, resume)
	if err != nil {
		return "", err
	}
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.New("cannot open Claude result stream")
	}
	if err := process.start(cmd, stream); err != nil {
		return "", err
	}
	stop := context.AfterFunc(ctx, func() { stream.Close() })
	defer stop()
	session, completed, parseErr := readClaudeEvents(stream, resume, process.session, process.claudeTool, process.question)
	if parseErr != nil && !errors.Is(parseErr, errTurnFailed) {
		stream.Close()
		_ = cmd.Process.Kill()
	}
	exit, err := process.wait(cmd)
	switch {
	case err != nil:
		return session, fmt.Errorf("record Claude process exit: %w", err)
	case ctx.Err() != nil:
		return session, ctx.Err()
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
