package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"forgeos/internal/review"
	"forgeos/internal/store"
)

func claudeExecutable() (string, error) {
	name := os.Getenv("FORGEOS_CLAUDE_BIN")
	if name == "" {
		name = "claude"
		if runtime.GOOS == "windows" {
			name = "claude.exe"
		}
	}
	bin, err := exec.LookPath(name)
	if err != nil || runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(bin), ".exe") {
		return "", errors.New("native Claude Code executable unavailable")
	}
	return bin, nil
}
func claudeRoleJSON(ctx context.Context, contract, prompt string, limit int64, dest any, process processLog) (string, error) {
	var dir, resume string
	var err error
	if process.roleNative != nil {
		dir, resume = process.roleNative.Workspace, process.roleNative.Resume
		prompt, contract, err = process.freezeRoleInput(prompt, contract)
	} else {
		dir, err = os.MkdirTemp("", "forgeos-readonly-")
		if err == nil {
			defer os.RemoveAll(dir)
		}
	}
	if err != nil {
		return "", err
	}
	bin, err := claudeExecutable()
	if err != nil {
		return "", err
	}
	return claudeStructured(ctx, bin, dir, contract, prompt, resume, limit, dest, process, review.ClaudeProfile())
}

func claudeStructured(ctx context.Context, bin, dir, contract, fullPrompt, resume string, limit int64, dest any, process processLog, profile store.AgentProfile) (session string, resultErr error) {
	defer func() { runtimeAgent(ctx).finish(ctx, "claude", session, resultErr) }()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	agent := runtimeAgent(ctx)
	streaming := process.db != nil || agent != nil
	format := "json"
	if streaming {
		format = "stream-json"
	}
	cmd := exec.CommandContext(ctx, bin, "-p", "--output-format", format, "--json-schema", contract, "--tools", strings.Join(*profile.Tools, ","), "--strict-mcp-config",
		"--disable-slash-commands", "--no-chrome", "--permission-mode", profile.Permission, "--setting-sources", "user", "--settings", `{"disableAllHooks":true}`)
	skillServer, err := selectedSkillServer(ctx)
	if err != nil {
		return "", err
	}
	cmd.Args = skillServer.claudeArgs(agent.modelArgs("claude", cmd.Args))
	if streaming {
		cmd.Args = append(cmd.Args, "--verbose")
	}
	if resume != "" {
		cmd.Args = append(cmd.Args, "--resume", resume)
	}
	cmd.Dir, cmd.WaitDelay = dir, 3*time.Second
	for _, entry := range harnessEnv() {
		if !strings.HasPrefix(strings.ToUpper(entry), "CODEX_HOME=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "CLAUDE_CODE_GIT_BASH_PATH"} {
		if value := os.Getenv(key); value != "" {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	cmd.Stdin = strings.NewReader(fullPrompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = process.start(cmd, stdout); err != nil {
		return "", errors.New("could not start Claude Code")
	}
	closeStream := context.AfterFunc(ctx, func() { stdout.Close() })
	defer closeStream()
	var raw []byte
	var observedSession string
	var completed bool
	if !streaming {
		raw, err = io.ReadAll(io.LimitReader(stdout, (1<<20)+1))
	} else {
		observedSession, completed, err = readClaudeEvents(stdout, resume, process.session, func(kind string, body []byte) error {
			if kind == "result" {
				if len(body) > 1<<20 {
					return errors.New("Claude structured turn result too large")
				}
				raw = append([]byte(nil), body...)
				return nil
			}
			return process.claudeActivity(kind, body)
		}, nil, nil, agent.observeModel)
		if err == nil && !completed {
			err = errors.New("Claude structured turn has no terminal result")
		}
	}
	if err != nil || len(raw) > 1<<20 {
		cancel()
	}
	exit, journalErr := process.wait(cmd)
	if errors.Is(err, errSessionMissing) && journalErr == nil {
		return "", errSessionMissing
	}
	if exit != nil && journalErr == nil && !completed && nativeSessionID.MatchString(observedSession) {
		return observedSession, errTurnInterrupted
	}
	if exit != nil || journalErr != nil || err != nil || len(raw) > 1<<20 {
		return "", errors.New("Claude structured turn did not finish with a bounded result")
	}
	var result struct {
		Subtype string          `json:"subtype"`
		Failed  bool            `json:"is_error"`
		Session string          `json:"session_id"`
		Output  json.RawMessage `json:"structured_output"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Subtype != "success" || result.Failed || len(result.Output) == 0 || int64(len(result.Output)) > limit || string(result.Output) == "null" || json.Unmarshal(result.Output, dest) != nil {
		return observedSession, errors.New("invalid Claude structured output")
	}
	return result.Session, nil
}
