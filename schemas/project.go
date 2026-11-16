package schemas

import (
	_ "embed"
	"errors"
	"io/fs"
	"strings"
	"unicode"
)

//go:embed project.schema.json
var projectJSON []byte
var projectSchema = cachedSchema("urn:forgeos:project:v1", projectJSON)

type SecretRequirement struct {
	Environment string `json:"environment"`
	Name        string `json:"name"`
	Risk        string `json:"risk"`
}

type ProjectConfig struct {
	Execution struct {
		DefaultHarness string `json:"default_harness"`
	} `json:"execution"`
	SecretRequirements []SecretRequirement         `json:"secret_requirements"`
	LocalEnvironments  map[string]LocalEnvironment `json:"local_environments,omitempty"`
}

type LocalEnvironment struct {
	StartScript    string `json:"start_script"`
	Port           int    `json:"port"`
	HealthPath     string `json:"health_path"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func ParseProject(raw []byte) (ProjectConfig, error) {
	project := ProjectConfig{SecretRequirements: []SecretRequirement{}}
	if err := decodeConfig(raw, projectSchema, &project); err != nil {
		return ProjectConfig{}, err
	}
	for _, env := range project.LocalEnvironments {
		if !fs.ValidPath(env.StartScript) || env.StartScript == "." || strings.ContainsAny(env.StartScript, "\\:") || strings.IndexFunc(env.StartScript, unicode.IsControl) >= 0 {
			return ProjectConfig{}, errors.New("start_script must be a repository-relative file")
		}
	}
	seen := map[[2]string]bool{}
	for _, r := range project.SecretRequirements {
		key := [2]string{r.Environment, r.Name}
		if seen[key] {
			return ProjectConfig{}, errors.New("duplicate secret name in one environment")
		}
		seen[key] = true
	}
	return project, nil
}

func ProjectHarness(raw []byte) (string, error) {
	project, err := ParseProject(raw)
	return project.Execution.DefaultHarness, err
}
