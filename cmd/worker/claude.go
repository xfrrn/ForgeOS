package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type claudeCLI struct {
	bin, version string
}

var claudeVersion = regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+) \(Claude Code\)$`)
var claudeSession = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

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
	if err != nil || !bytes.Contains(help, []byte("--restricted")) || !bytes.Contains(help, []byte("--safe-mode")) {
		return claudeCLI{}, errors.New("Claude Code must support --restricted and --safe-mode")
	}
	out, err := command("--version")
	version := claudeVersion.FindStringSubmatch(strings.TrimSpace(string(out)))
	if err != nil || len(version) != 2 {
		return claudeCLI{}, errors.New("cannot identify the Claude Code version")
	}
	return claudeCLI{bin, version[1]}, nil
}

func (c claudeCLI) turn(ctx context.Context, workspace, prompt, resume string) (string, error) {
	args := []string{"-p", "--restricted", "--safe-mode", "--strict-mcp-config", "--no-chrome", "--disable-slash-commands",
		"--permission-mode", "acceptEdits", "--tools", "Read,Glob,Grep,Edit,Write", "--output-format", "json"}
	config := os.Getenv("CLAUDE_CONFIG_DIR")
	if config == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		config = filepath.Join(home, ".claude")
	}
	if !filepath.IsAbs(config) {
		return "", errors.New("Claude configuration directory must be absolute")
	}
	// Restricted mode ignores user settings by default, including the user's model gateway.
	settings := filepath.Join(config, "settings.json")
	if _, err := os.Stat(settings); err == nil {
		args = append(args, "--settings", settings)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("cannot read Claude user settings")
	}
	if resume != "" {
		if !claudeSession.MatchString(resume) {
			return "", errors.New("invalid Claude session ID")
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
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.New("cannot open Claude result stream")
	}
	if err = cmd.Start(); err != nil {
		return "", errors.New("could not start Claude Code")
	}
	stop := context.AfterFunc(ctx, func() { stream.Close() })
	defer stop()
	const limit = 4 << 20
	data, err := io.ReadAll(io.LimitReader(stream, limit+1))
	if err != nil || len(data) > limit {
		stream.Close()
		_ = cmd.Process.Kill()
	}
	exit := cmd.Wait()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	var result struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		IsError bool   `json:"is_error"`
		Session string `json:"session_id"`
	}
	if err != nil || len(data) > limit || json.Unmarshal(data, &result) != nil || result.Type != "result" || !claudeSession.MatchString(result.Session) || (resume != "" && result.Session != resume) {
		return "", errors.New("invalid Claude Code result or session")
	}
	if exit != nil || result.IsError || result.Subtype != "success" {
		return result.Session, errors.New("Claude Code turn failed; check the node login and model availability")
	}
	return result.Session, nil
}
