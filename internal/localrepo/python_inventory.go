package localrepo

import (
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

var distribution = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)
var normalizeDistribution = regexp.MustCompile(`[._-]+`)
var namedRequirement = regexp.MustCompile(`^([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)[[:space:]]*(?:$|\[|@|;|\(|(?:===|==|!=|~=|<=?|>=?))`)
var pipCommentOrOption = regexp.MustCompile(`[ \t]+(?:#|--)`)
var pipSourceOption = regexp.MustCompile(`^(?:-[rce]|--(?:requirement|constraint|editable)(?:=|$))`)
var archiveName = regexp.MustCompile(`(?i)\.(whl|zip|tar\.(gz|bz2|xz)|tgz)$`)

func (f *InventoryFile) readPython(data []byte, deps map[string]bool) bool {
	if !utf8.Valid(data) {
		return false
	}
	add := func(requirement string) {
		match := namedRequirement.FindStringSubmatch(strings.TrimSpace(requirement))
		if match == nil {
			f.UnresolvedInputs++
			return
		}
		deps[normalizeDistribution.ReplaceAllString(strings.ToLower(match[1]), "-")] = true
	}
	if strings.EqualFold(path.Base(f.Path), "requirements.txt") {
		for _, line := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\\\n", ""), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if strings.HasPrefix(line, "-") {
				if pipSourceOption.MatchString(strings.Fields(line)[0]) {
					f.UnresolvedInputs++
				}
				continue
			}
			if cut := pipCommentOrOption.FindStringIndex(line); cut != nil {
				line = line[:cut[0]]
			}
			if distribution.MatchString(line) && archiveName.MatchString(line) {
				f.UnresolvedInputs++
				continue
			}
			add(line)
		}
		return true
	}
	var m struct {
		Project struct {
			Dependencies []string
			Optional     map[string][]string `toml:"optional-dependencies"`
			Scripts      map[string]string
			GUI          map[string]string `toml:"gui-scripts"`
			Dynamic      []string
		}
		Build  struct{ Requires []string } `toml:"build-system"`
		Groups map[string][]any            `toml:"dependency-groups"`
		Tool   struct {
			Poetry struct {
				Dependencies map[string]any
				Dev          map[string]any `toml:"dev-dependencies"`
				Group        map[string]struct{ Dependencies map[string]any }
				Scripts      map[string]any
			}
		}
	}
	if _, err := toml.Decode(string(data), &m); err != nil {
		return false
	}
	for _, req := range m.Project.Dependencies {
		add(req)
	}
	for _, req := range m.Build.Requires {
		add(req)
	}
	for _, group := range m.Project.Optional {
		for _, req := range group {
			add(req)
		}
	}
	for _, group := range m.Groups {
		for _, entry := range group {
			if req, ok := entry.(string); ok {
				add(req)
			} else if ref, ok := entry.(map[string]any); !ok || ref["include-group"] == nil {
				f.UnresolvedInputs++
			}
		}
	}
	poetry := m.Tool.Poetry
	groups := []map[string]any{poetry.Dependencies, poetry.Dev}
	for _, group := range poetry.Group {
		groups = append(groups, group.Dependencies)
	}
	for _, group := range groups {
		for name := range group {
			if name != "python" && distribution.MatchString(name) {
				add(name)
			}
		}
	}
	for _, field := range m.Project.Dynamic {
		if slices.Contains([]string{"dependencies", "optional-dependencies", "scripts", "gui-scripts"}, field) && !slices.Contains(f.DynamicFields, field) {
			f.DynamicFields = append(f.DynamicFields, field)
		}
	}
	sort.Strings(f.DynamicFields)
	names := map[string]bool{}
	for _, group := range []map[string]string{m.Project.Scripts, m.Project.GUI} {
		for name, target := range group {
			if target != "" && scriptName.MatchString(name) {
				names[name] = true
			}
		}
	}
	for name := range poetry.Scripts {
		if scriptName.MatchString(name) {
			names[name] = true
		}
	}
	for name := range names {
		f.Scripts = append(f.Scripts, name)
	}
	return true
}
