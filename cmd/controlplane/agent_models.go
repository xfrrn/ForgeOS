package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"forgeos/internal/agents"
)

type modelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Only publish model identifiers and labels. The source files can also contain
// account metadata, credentials, and model instructions.
func readHarnessCatalog(envKey, defaultDir, file string, dest any) {
	dir := os.Getenv(envKey)
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dir = filepath.Join(home, defaultDir)
	}
	f, err := os.Open(filepath.Join(dir, file))
	if err != nil {
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err == nil && len(raw) < 8<<20 {
		_ = json.Unmarshal(raw, dest)
	}
}

func agentModels() map[string][]modelOption {
	var cache struct {
		Models []struct {
			ID         string `json:"slug"`
			Name       string `json:"display_name"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	readHarnessCatalog("CODEX_HOME", ".codex", "models_cache.json", &cache)
	models := map[string][]modelOption{"codex": {}, "claude": {}}
	seen := map[string]bool{}
	for _, m := range cache.Models {
		if m.Visibility != "list" || m.ID == "" || !agents.ValidModel(m.ID) || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		label := m.Name
		if label == "" || len(label) > 128 {
			label = m.ID
		}
		models["codex"] = append(models["codex"], modelOption{m.ID, label})
	}
	var settings struct {
		Env map[string]string `json:"env"`
	}
	readHarnessCatalog("CLAUDE_CONFIG_DIR", ".claude", "settings.json", &settings)
	for _, alias := range []string{"opus", "sonnet", "haiku", "fable"} {
		label := alias
		key := "ANTHROPIC_DEFAULT_" + strings.ToUpper(alias) + "_MODEL"
		mapped := os.Getenv(key)
		if mapped == "" {
			mapped = settings.Env[key]
		}
		if mapped != "" && agents.ValidModel(mapped) {
			label = alias + " · " + mapped
		}
		models["claude"] = append(models["claude"], modelOption{alias, label})
	}
	return models
}
