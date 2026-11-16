package localrepo

import (
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

type ManifestSource struct {
	Path string `json:"path"`
	Blob string `json:"blob"`
}

type cargoTables struct {
	Dependencies map[string]any
	Dev          map[string]any `toml:"dev-dependencies"`
	Build        map[string]any `toml:"build-dependencies"`
	LegacyDev    map[string]any `toml:"dev_dependencies"`
	LegacyBuild  map[string]any `toml:"build_dependencies"`
}

type cargoManifest struct {
	cargoTables
	Package *struct {
		Name      string
		Workspace string
	}
	Workspace *struct {
		Dependencies     map[string]any
		Members, Exclude []string
	}
	Target map[string]cargoTables
}

var crateName = regexp.MustCompile(`^[\p{L}\p{N}_-]+$`)

func readCargo(data []byte) *cargoManifest {
	var m cargoManifest
	if err := toml.Unmarshal(data, &m); err != nil || (m.Package == nil && m.Workspace == nil) || (m.Package != nil && !crateName.MatchString(m.Package.Name)) {
		return nil
	}
	return &m
}

func cratePackage(alias string, decl any) string {
	if !crateName.MatchString(alias) {
		return ""
	}
	switch d := decl.(type) {
	case string:
		return alias
	case map[string]any:
		if _, inherits := d["workspace"]; inherits {
			return ""
		}
		if d["version"] == nil && d["path"] == nil && d["git"] == nil {
			return ""
		}
		if name, has := d["package"]; has {
			actual, ok := name.(string)
			if !ok || !crateName.MatchString(actual) {
				return ""
			}
			return actual
		}
		return alias
	}
	return ""
}

func cargoRoot(file string, m *cargoManifest, manifests map[string]*cargoManifest) string {
	dir := path.Dir(file)
	allows := func(root string, ws *cargoManifest) bool {
		if ws == nil || ws.Workspace == nil {
			return false
		}
		contains := func(entry string) bool {
			p := path.Join(path.Dir(root), strings.ReplaceAll(entry, "\\", "/"))
			return p == "." || file == p || strings.HasPrefix(file, p+"/")
		}
		return !slices.ContainsFunc(ws.Workspace.Exclude, contains) || slices.ContainsFunc(ws.Workspace.Members, contains)
	}
	if m.Package != nil && m.Package.Workspace != "" {
		ref := strings.ReplaceAll(m.Package.Workspace, "\\", "/")
		if path.IsAbs(ref) || strings.Contains(ref, ":") {
			return ""
		}
		root := path.Join(dir, ref, "Cargo.toml")
		if strings.HasPrefix(root, "../") {
			return ""
		}
		if allows(root, manifests[root]) {
			return root
		}
		return ""
	}
	if m.Workspace != nil {
		return file
	}
	for {
		root := path.Join(dir, "Cargo.toml")
		if found, ok := manifests[root]; ok {
			if found == nil {
				return ""
			}
			if allows(root, found) {
				return root
			}
		}
		if dir == "." {
			return ""
		}
		dir = path.Dir(dir)
	}
}

func resolveCargo(files []InventoryFile, manifests map[string]*cargoManifest) {
	if len(manifests) == 0 {
		return
	}
	blobs := map[string]string{}
	for _, f := range files {
		if f.Kind == "cargo" {
			blobs[f.Path] = f.Blob
		}
	}
	for i := range files {
		f := &files[i]
		m := manifests[f.Path]
		if m == nil {
			continue
		}
		if m.Workspace != nil {
			names := map[string]bool{}
			for alias, decl := range m.Workspace.Dependencies {
				if name := cratePackage(alias, decl); name != "" {
					names[name] = true
				} else {
					f.UnresolvedInputs++
				}
			}
			f.WorkspaceDependencies = slices.Sorted(maps.Keys(names))
		}
		root := cargoRoot(f.Path, m, manifests)
		names := map[string]bool{}
		collect := func(t cargoTables) {
			if t.Dev == nil {
				t.Dev = t.LegacyDev
			}
			if t.Build == nil {
				t.Build = t.LegacyBuild
			}
			for _, group := range []map[string]any{t.Dependencies, t.Dev, t.Build} {
				for alias, decl := range group {
					if details, ok := decl.(map[string]any); ok && details["workspace"] == true {
						decl = nil
						if root != "" {
							decl = manifests[root].Workspace.Dependencies[alias]
							f.WorkspaceSource = &ManifestSource{Path: root, Blob: blobs[root]}
						}
					}
					if name := cratePackage(alias, decl); name != "" {
						names[name] = true
					} else {
						f.UnresolvedInputs++
					}
				}
			}
		}
		collect(m.cargoTables)
		for _, target := range m.Target {
			collect(target)
		}
		f.Dependencies = slices.Sorted(maps.Keys(names))
	}
}
