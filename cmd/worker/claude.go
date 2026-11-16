package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

func (c claudeCLI) command(ctx context.Context, workspace, prompt, resume string) (*exec.Cmd, error) {
	args := []string{"-p", "--restricted", "--safe-mode", "--strict-mcp-config", "--no-chrome", "--disable-slash-commands",
		"--permission-mode", "acceptEdits", "--tools", "Read,Glob,Grep,Edit,Write", "--verbose", "--output-format", "stream-json"}
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
		if !claudeSession.MatchString(resume) {
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

var errClaudeInterrupted = errors.New("Claude Code exited before the turn result")
var errClaudeTurnFailed = errors.New("Claude Code reported a failed turn")

func readClaudeEvents(stream io.Reader, expected string, confirmed func(string) error) (session string, completed bool, err error) {
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 16<<10), 4<<20)
	var outcome error
	for scanner.Scan() {
		var event struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			IsError bool   `json:"is_error"`
			Session string `json:"session_id"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			return session, completed, errors.New("invalid Claude Code event")
		}
		switch {
		case event.Type == "system" && event.Subtype == "init":
			if session != "" || !claudeSession.MatchString(event.Session) || (expected != "" && event.Session != expected) {
				return session, completed, errors.New("unexpected Claude Code session")
			}
			if err := confirmed(event.Session); err != nil {
				return "", false, err
			}
			session = event.Session
		case event.Type == "result":
			if completed || session == "" || session != event.Session {
				return session, false, errors.New("mismatched Claude Code result")
			}
			completed = true
			if event.IsError || event.Subtype != "success" {
				outcome = errClaudeTurnFailed
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return session, completed, err
	}
	return session, completed, outcome
}

func (w *worker) runClaudeProcess(ctx context.Context, job store.Job, prompt, resume string) (string, error) {
	cmd, err := w.claude.command(ctx, job.Workspace, prompt, resume)
	if err != nil {
		return "", err
	}
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.New("cannot open Claude result stream")
	}
	if err = cmd.Start(); err != nil {
		return "", errors.New("could not start Claude Code")
	}
	key := sha256.Sum256([]byte(job.LeaseToken))
	local := context.WithoutCancel(ctx)
	record, err := w.journal.ExecContext(local, `INSERT INTO harness_processes(lease_hash,harness,version,pid) VALUES(?,'claude',?,?)`, key[:], w.claude.version, cmd.Process.Pid)
	var instance int64
	if err == nil {
		instance, err = record.LastInsertId()
	}
	if err != nil {
		stream.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", fmt.Errorf("record Claude process: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { stream.Close() })
	defer stop()
	session, completed, parseErr := readClaudeEvents(stream, resume, func(id string) error {
		_, err := w.journal.ExecContext(local, "UPDATE harness_processes SET session_id=? WHERE instance=?", id, instance)
		return err
	})
	if parseErr != nil && !errors.Is(parseErr, errClaudeTurnFailed) {
		stream.Close()
		_ = cmd.Process.Kill()
	}
	exit := cmd.Wait()
	_, err = w.journal.ExecContext(local, "UPDATE harness_processes SET exited_at=CURRENT_TIMESTAMP,exit_code=? WHERE instance=?", cmd.ProcessState.ExitCode(), instance)
	switch {
	case err != nil:
		return session, fmt.Errorf("record Claude process exit: %w", err)
	case ctx.Err() != nil:
		return session, ctx.Err()
	case parseErr != nil:
		return session, parseErr
	case exit != nil && !completed && session != "":
		return session, errClaudeInterrupted
	case exit != nil || !completed:
		return session, errors.New("Claude Code did not complete the turn")
	}
	return session, nil
}

func (w *worker) claudeTurn(ctx context.Context, job store.Job, prompt, previous, start string, identity []byte) (string, error) {
	session, err := w.runClaudeProcess(ctx, job, prompt, previous)
	if !errors.Is(err, errClaudeInterrupted) || ctx.Err() != nil {
		return session, err
	}
	status, err := w.post(ctx, fmt.Sprintf("/internal/executions/%d/renew", job.ExecutionID), map[string]string{"lease_token": job.LeaseToken}, nil)
	if err != nil || status != http.StatusNoContent {
		return session, errors.New("cannot confirm lease for interrupted Claude session")
	}
	tip, err := checkedWorktreeTip(ctx, job.Workspace, identity)
	if err != nil || tip != start {
		return session, errors.New("cannot confirm original Claude worktree and starting commit")
	}
	prompt = "The previous process was interrupted. Inspect existing edits in this worktree and finish only the remaining authorized work.\n" + prompt
	_, err = w.runClaudeProcess(ctx, job, prompt, session)
	return session, err
}
