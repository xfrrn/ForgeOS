package runtimeinfo

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

type Space struct {
	Available uint64 `json:"available"`
	Total     uint64 `json:"total"`
}

type Host struct {
	SampledAt    time.Time `json:"sampled_at"`
	CPUPercent   *uint64   `json:"cpu_percent,omitempty"`
	Memory       *Space    `json:"memory,omitempty"`
	Repositories *Space    `json:"repositories,omitempty"`
	Managed      *Space    `json:"managed,omitempty"`
	Workspace    *Space    `json:"workspace,omitempty"`
}

type CLI struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

type Tools struct {
	CheckedAt time.Time `json:"checked_at"`
	CLI       []CLI     `json:"cli"`
	Docker    string    `json:"docker"`
}

type Snapshot struct {
	Host  *Host  `json:"host,omitempty"`
	Tools *Tools `json:"tools,omitempty"`
}

type Paths struct {
	Repositories string
	Managed      string
	Workspace    string
}

type Harness struct{ Path, Version string }

type Monitor struct {
	host  atomic.Pointer[Host]
	tools atomic.Pointer[Tools]
}

// Start collects independently of heartbeat and HTTP request handling.
func Start(ctx context.Context, paths Paths, harnesses map[string]Harness) *Monitor {
	m := new(Monitor)
	go func() {
		var previous *cpuTimes
		tick(ctx, 15*time.Second, func() {
			current, memory := hostResources()
			var percent *uint64
			if current != nil && previous != nil && current.total > previous.total && current.busy >= previous.busy {
				total, busy := current.total-previous.total, current.busy-previous.busy
				if busy <= total {
					value := uint64(float64(busy) * 100 / float64(total))
					percent = &value
				}
			}
			previous = current
			m.host.Store(&Host{SampledAt: time.Now().UTC(), CPUPercent: percent, Memory: memory,
				Repositories: diskSpace(paths.Repositories), Managed: diskSpace(paths.Managed), Workspace: diskSpace(paths.Workspace)})
		})
	}()
	go tick(ctx, 5*time.Minute, func() { m.tools.Store(probeTools(ctx, harnesses)) })
	return m
}

func (m *Monitor) Snapshot() Snapshot {
	return Snapshot{Host: m.host.Load(), Tools: m.tools.Load()}
}

func tick(ctx context.Context, interval time.Duration, run func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		run()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type cpuTimes struct{ total, busy uint64 }

type toolCommand struct{ name, argument string }

var toolCommands = []toolCommand{
	{"codex", "--version"}, {"claude", "--version"}, {"git", "--version"}, {"gh", "--version"},
	{"docker", "--version"}, {"node", "--version"}, {"npm", "--version"}, {"python", "--version"}, {"go", "version"},
}

var versionPattern = regexp.MustCompile(`[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:[-+.][0-9A-Za-z-]+)*`)

func probeTools(ctx context.Context, harnesses map[string]Harness) *Tools {
	tools := &Tools{Docker: "not_found"}
	for _, command := range toolCommands {
		cli := CLI{Name: command.name, Status: "not_found"}
		harness := harnesses[cli.Name]
		path := harness.Path
		if path == "" {
			path = toolPath(cli.Name)
		} else {
			path, _ = exec.LookPath(path)
		}
		if path != "" {
			cli.Status = "found"
			if cli.Name == "docker" {
				tools.Docker = "unreachable"
			}
			dir, executable, args := versionCommand(command, path)
			if executable != "" {
				out, err := probe(ctx, dir, executable, args...)
				cli.Status = "probe_failed"
				if err == nil {
					cli.Status = "responds"
					cli.Version = readVersion(out)
					if cli.Version != "" && cli.Version == harness.Version {
						cli.Status = "supported"
					}
				}
				if cli.Name == "docker" && cli.Status == "responds" {
					if out, err := probe(ctx, dir, path, "version", "--format", "{{.Server.Version}}"); err == nil && readVersion(out) != "" {
						tools.Docker = "reachable"
					}
				}
			}
		}
		tools.CLI = append(tools.CLI, cli)
	}
	tools.CheckedAt = time.Now().UTC()
	return tools
}

func versionCommand(command toolCommand, path string) (string, string, []string) {
	if runtime.GOOS == "windows" {
		if command.name == "npm" && strings.EqualFold(filepath.Base(path), "npm.cmd") {
			// Run this exact shim, including its npm prefix selection. Keep directory
			// names out of the shell command, even when the install path contains "&".
			return filepath.Dir(path), filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"),
				[]string{"/d", "/c", `.\npm.cmd`, "--version"}
		}
		if !strings.EqualFold(filepath.Ext(path), ".exe") ||
			command.name == "python" && strings.Contains(strings.ToLower(path), `\microsoft\windowsapps\`) {
			return "", "", nil
		}
	}
	return os.TempDir(), path, []string{command.argument}
}

func toolPath(name string) string {
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath(name + ".exe"); err == nil {
			return path
		}
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return path
}

func readVersion(out string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	version := versionPattern.FindString(line)
	if len(version) > 64 {
		return ""
	}
	return version
}

func probe(ctx context.Context, dir, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.Env = probeEnv()
	cmd.WaitDelay = 250 * time.Millisecond
	var capture probeOutput
	cmd.Stdout = &capture
	err := cmd.Run()
	return capture.buffer.String(), err
}

func probeEnv() []string {
	// These read-only commands need executable discovery and per-user runtime
	// configuration. Service tokens and database connection strings stay in ForgeOS.
	env := []string{"GOTOOLCHAIN=local"}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "PATH", "PATHEXT", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "SYSTEMROOT", "WINDIR",
			"COMSPEC", "TEMP", "TMP", "TMPDIR", "XDG_CONFIG_HOME", "XDG_RUNTIME_DIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR",
			"DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_CERT_PATH", "DOCKER_TLS_VERIFY", "DOCKER_TLS", "SSH_AUTH_SOCK":
			env = append(env, entry)
		}
	}
	return env
}

type probeOutput struct{ buffer bytes.Buffer }

func (o *probeOutput) Write(p []byte) (int, error) {
	_, _ = o.buffer.Write(p[:min(len(p), 1024-o.buffer.Len())])
	return len(p), nil
}

func (s Snapshot) Valid() bool {
	if s.Host != nil {
		if s.Host.SampledAt.IsZero() || s.Host.CPUPercent != nil && *s.Host.CPUPercent > 100 {
			return false
		}
		for _, space := range []*Space{s.Host.Memory, s.Host.Repositories, s.Host.Managed, s.Host.Workspace} {
			if space != nil && (space.Total == 0 || space.Available > space.Total) {
				return false
			}
		}
	}
	if s.Tools == nil {
		return true
	}
	if s.Tools.CheckedAt.IsZero() || len(s.Tools.CLI) != len(toolCommands) {
		return false
	}
	switch s.Tools.Docker {
	case "not_found", "reachable", "unreachable":
	default:
		return false
	}
	for i, cli := range s.Tools.CLI {
		if cli.Name != toolCommands[i].name || len(cli.Version) > 64 || cli.Version != "" && readVersion(cli.Version) != cli.Version {
			return false
		}
		switch cli.Status {
		case "not_found", "found", "responds", "probe_failed":
		case "supported":
			if cli.Version == "" || cli.Name != "codex" && cli.Name != "claude" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
