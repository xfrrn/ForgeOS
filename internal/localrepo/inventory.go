package localrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"forgeos/schemas"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

type InventoryFile struct {
	Path               string                              `json:"path"`
	Blob               string                              `json:"blob"`
	Kind               string                              `json:"kind"`
	Dependencies       []string                            `json:"dependencies,omitempty"`
	Scripts            []string                            `json:"scripts,omitempty"`
	SecretRequirements *[]schemas.SecretRequirement        `json:"secret_requirements,omitempty"`
	LocalEnvironments  map[string]schemas.LocalEnvironment `json:"local_environments,omitempty"`
	Problem            string                              `json:"problem,omitempty"`
}

var objectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
var npmName = regexp.MustCompile(`^(?:@[a-zA-Z0-9][a-zA-Z0-9._-]*/)?[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var scriptName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)

func inventoryKind(file string) string {
	switch strings.ToLower(path.Base(file)) {
	case "readme", "readme.md", "readme.rst", "agents.md", "contributing.md", "file_index.md":
		return "guide"
	case "go.mod":
		return "go"
	case "package.json":
		return "node"
	case "pyproject.toml", "requirements.txt", "cargo.toml", "pom.xml", "build.gradle", "build.gradle.kts", "gemfile", "composer.json":
		return "manifest"
	case "go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "uv.lock", "cargo.lock", "poetry.lock", "gemfile.lock", "composer.lock":
		return "lock"
	case "makefile", "dockerfile", "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
		return "runtime"
	case "project.yaml", "validation.yaml", "acceptance.yaml":
		return "forgeos"
	}
	return ""
}

// Inventory reads committed Git objects only. It never checks out files or evaluates package scripts.
func Inventory(ctx context.Context, repo, commit string) ([]InventoryFile, error) {
	if !objectID.MatchString(commit) {
		return nil, errors.New("invalid commit")
	}
	obj, err := Git(ctx, repo, "cat-file", "-t", commit)
	if err != nil || string(obj) != "commit\n" {
		return nil, errors.New("verified commit unavailable")
	}
	raw, large, err := LimitedGit(ctx, repo, 16<<20, "ls-tree", "-r", "-z", commit)
	if err != nil || large {
		return nil, errors.New("cannot read complete repository tree within 16 MiB")
	}
	files := []InventoryFile{}
	readBudget := 8 << 20
	for _, entry := range bytes.Split(raw, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		header, file, ok := strings.Cut(string(entry), "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || !objectID.MatchString(fields[2]) {
			return nil, errors.New("invalid repository tree entry")
		}
		kind := inventoryKind(file)
		if kind == "" || !(fields[0] == "100644" || fields[0] == "100755") || fields[1] != "blob" {
			continue
		}
		if !utf8.ValidString(file) || strings.IndexFunc(file, unicode.IsControl) >= 0 {
			continue
		}
		dirs := "/" + path.Dir(file) + "/"
		if strings.Contains(dirs, "/node_modules/") || strings.Contains(dirs, "/vendor/") {
			continue
		}
		if len(files) >= 2000 {
			return nil, errors.New("repository exceeds 2000 inventory files")
		}
		c := InventoryFile{Path: file, Blob: fields[2], Kind: kind}
		if file == "project.yaml" {
			if err := c.readProject(ctx, repo); err != nil {
				return nil, err
			}
		} else if kind == "node" || kind == "go" {
			if readBudget <= 0 {
				c.Problem = "read_budget"
			} else {
				data, large, err := LimitedGit(ctx, repo, min(256<<10, readBudget), "cat-file", "blob", c.Blob)
				if err != nil {
					return nil, errors.New("cannot read manifest object")
				}
				readBudget -= len(data)
				if large {
					c.Problem = "size_limit"
				} else {
					c.readManifest(data)
				}
			}
		}
		files = append(files, c)
	}
	return files, nil
}

func (f *InventoryFile) readProject(ctx context.Context, repo string) error {
	data, large, err := LimitedGit(ctx, repo, schemas.MaxValidationBytes, "cat-file", "blob", f.Blob)
	if err != nil {
		return errors.New("cannot read project configuration")
	}
	if large {
		f.Problem = "size_limit"
		return nil
	}
	project, err := schemas.ParseProject(data)
	if err != nil {
		f.Problem = "invalid_project"
		return nil
	}
	f.SecretRequirements = &project.SecretRequirements
	f.LocalEnvironments = project.LocalEnvironments
	return nil
}

func (f *InventoryFile) readManifest(data []byte) {
	deps := map[string]bool{}
	if f.Kind == "go" {
		m, err := modfile.ParseLax(f.Path, data, nil)
		if err != nil || m.Module == nil {
			f.Problem = "invalid_manifest"
			return
		}
		for _, d := range m.Require {
			if module.CheckPath(d.Mod.Path) == nil {
				deps[d.Mod.Path] = true
			}
		}
	} else {
		var m struct{ Scripts, Dependencies, DevDependencies, PeerDependencies, OptionalDependencies map[string]json.RawMessage }
		if err := json.Unmarshal(data, &m); err != nil || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			f.Problem = "invalid_manifest"
			return
		}
		for name, body := range m.Scripts {
			if scriptName.MatchString(name) && len(body) > 0 && body[0] == '"' {
				f.Scripts = append(f.Scripts, name)
			}
		}
		sort.Strings(f.Scripts)
		for _, group := range []map[string]json.RawMessage{m.Dependencies, m.DevDependencies, m.PeerDependencies, m.OptionalDependencies} {
			for name := range group {
				if len(name) <= 214 && npmName.MatchString(name) {
					deps[name] = true
				}
			}
		}
	}
	for name := range deps {
		f.Dependencies = append(f.Dependencies, name)
	}
	sort.Strings(f.Dependencies)
}
