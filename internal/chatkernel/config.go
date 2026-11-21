// Package chatkernel connects the Codex agent loop to a project's Chat Completions provider.
package chatkernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
)

type ToolServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Tools   []string `json:"tools"`
}
type Config struct {
	TaskTool TaskToolCall `json:"-"`
	// Preview receives only the top-level answer text as it becomes available.
	Preview        func(string) `json:"-"`
	ProjectID      int64        `json:"project_id"`
	provider       Provider
	providerStatus atomic.Int32
	Servers        map[string]ToolServer `json:"mcp_servers"`
}

var serverName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,31}$`)
var modelName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,127}$`)
var wireToolName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func ForProject(path string, project int64, p Provider) (*Config, error) {
	if !p.Valid() || project < 1 {
		return nil, errors.New("project model configuration required")
	}
	if path == "" {
		return &Config{ProjectID: project, provider: p}, nil
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("chat kernel configuration needs an absolute path")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg Config
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("unexpected extra chat configuration")
	}
	if cfg.ProjectID != project {
		return nil, errors.New("chat tools belong to a different project")
	}
	cfg.provider = p
	for name, server := range cfg.Servers {
		if !serverName.MatchString(name) || (name == "forgeos_skills" || name == "forgeos_tasks") || !filepath.IsAbs(server.Command) || len(server.Tools) == 0 {
			return nil, fmt.Errorf("invalid project MCP server %q", name)
		}
		for _, tool := range server.Tools {
			if tool == "" || strings.ContainsAny(tool, "\x00\r\n") {
				return nil, errors.New("invalid mounted tool name")
			}
		}
	}
	return &cfg, nil
}

func tomlString(value string) string { b, _ := json.Marshal(value); return string(b) }
func tomlList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = tomlString(item)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// Arguments retains the worker's explicit MCP configuration and adds only the project mounts.
func (c *Config) Arguments(base string) []string {
	selected := c.provider.Model
	args := []string{"--model", selected, "-c", `model_provider="forgeos_project"`, "-c", `model_providers.forgeos_project={name="ForgeOS",base_url=` + tomlString(base) + `,wire_api="responses",supports_websockets=false,request_max_retries=0,stream_max_retries=0}`}
	if c.TaskTool != nil {
		args = append(args, "-c", "mcp_servers.forgeos_tasks={url="+tomlString(base+"/tasks")+",enabled_tools="+tomlList([]string{ResolveTask, TaskProgress})+",required=true}")
	}
	for name, s := range c.Servers {
		args = append(args, "-c", "mcp_servers."+name+"={command="+tomlString(s.Command)+",args="+tomlList(s.Args)+",enabled_tools="+tomlList(s.Tools)+",required=true}")
	}
	return args
}
