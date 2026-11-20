package main

import (
	"context"
	"encoding/json"
	"errors"
	"forgeos/internal/skilllib"
	"os"
	"path/filepath"
)

const skillServer = "forgeos_skills"

type skillServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func selectedSkillServer(ctx context.Context) (*skillServerConfig, error) {
	a := runtimeAgent(ctx)
	if a == nil || len(a.Packages) == 0 {
		return nil, nil
	}
	bin, err := os.Executable()
	if err != nil {
		return nil, err
	}
	root, err := skilllib.Root()
	if err != nil {
		return nil, err
	}
	cfg := &skillServerConfig{Command: bin, Args: []string{"skill-files", root}}
	for _, ref := range a.Packages {
		dir, err := skilllib.Dir(ref.Key)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(filepath.Join(dir, "SKILL.md"))
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("selected ForgeOS Skill is unavailable on this worker: " + ref.Name)
		}
		cfg.Args = append(cfg.Args, ref.Key)
	}
	return cfg, nil
}
func (c *skillServerConfig) codexArgs(args []string) []string {
	if c == nil {
		return args
	}
	cmd, _ := json.Marshal(c.Command)
	arguments, _ := json.Marshal(c.Args)
	return append(args, "-c", "mcp_servers."+skillServer+"={command="+string(cmd)+",args="+string(arguments)+",required=true}")
}
func (c *skillServerConfig) claudeArgs(args []string) []string {
	if c == nil {
		return args
	}
	config, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{skillServer: c}})
	return append(args, "--mcp-config", string(config), "--allowedTools", "mcp__forgeos_skills__read")
}
