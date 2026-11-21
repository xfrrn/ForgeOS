package agents

import (
	_ "embed"
	"encoding/json"
	"forgeos/internal/skilllib"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

//go:embed plan_schema.json
var PlanSchema string

//go:embed catalog.json
var catalog []byte

type Definition struct {
	Key       string   `json:"key"`
	Role      string   `json:"role"`
	Title     string   `json:"title"`
	Harnesses []string `json:"harnesses"`
	Prompt    string   `json:"prompt"`
}

var Definitions []Definition
var byKey = map[string]Definition{}

func init() {
	if err := json.Unmarshal(catalog, &Definitions); err != nil {
		panic(err)
	}
	for _, d := range Definitions {
		byKey[d.Key] = d
	}
}
func Lookup(key string) (Definition, bool) { d, ok := byKey[key]; return d, ok }
func Builtin(key string) string            { return byKey[key].Prompt }

type Settings struct {
	Skills  []int64 `json:"skills,omitempty"`
	Prompt  string  `json:"prompt"`
	Harness string  `json:"harness"`
	Model   string  `json:"model"`
}

var modelID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/@-]{0,127}$`)

func ValidModel(id string) bool { return id == "" || modelID.MatchString(id) }
func (d Definition) Valid(s Settings) bool {
	return slices.Contains(d.Harnesses, s.Harness) && ValidModel(s.Model) && len(s.Prompt) <= 16000 && strings.TrimSpace(s.Prompt) != "" && !strings.ContainsRune(s.Prompt, 0)
}

type SkillRef struct {
	Name  string `json:"name"`
	Key   string `json:"key"`
	Entry string `json:"entry"`
}

type Config struct {
	Packages []SkillRef `json:"packages,omitempty"`

	Settings
	Key      string `json:"key"`
	Source   string `json:"source"`
	Revision int64  `json:"revision"`
}
type Assignment struct {
	Config
	Ticket string `json:"ticket"`
}

func (a *Assignment) Apply(prompt string) string {
	if a == nil {
		return prompt
	}
	instructions := a.Prompt
	for _, ref := range a.Packages {
		if dir, err := skilllib.Dir(ref.Key); err == nil {
			instructions = strings.ReplaceAll(instructions, ref.Entry, filepath.Join(dir, "SKILL.md"))
		}
	}
	original := Builtin(a.Key)
	if strings.HasPrefix(prompt, original) && original != "" {
		return instructions + strings.TrimPrefix(prompt, original)
	}
	// A later checkpoint can carry a continuation instruction instead of the initial prompt.
	return instructions + "\n\n" + prompt
}
