package localrepo

import (
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
)

type mavenID struct {
	Group    string `xml:"groupId"`
	Artifact string `xml:"artifactId"`
	Version  string `xml:"version"`
}
type mavenDependency struct {
	mavenID
	Scope string `xml:"scope"`
	Type  string `xml:"type"`
}
type mavenProperty struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}
type mavenSection struct {
	Properties mavenProperties   `xml:"properties"`
	Deps       []mavenDependency `xml:"dependencies>dependency"`
	Managed    []mavenDependency `xml:"dependencyManagement>dependencies>dependency"`
}
type mavenProperties struct {
	Entries []mavenProperty `xml:",any"`
}
type mavenProfile struct {
	ID string `xml:"id"`
	mavenSection
}
type mavenPOM struct {
	XMLName xml.Name `xml:"project"`
	mavenID
	mavenSection
	ModelVersion string `xml:"modelVersion"`
	Parent       *struct {
		mavenID
		Relative *string `xml:"relativePath"`
	} `xml:"parent"`
	Profiles []mavenProfile `xml:"profiles>profile"`
	blob     string
	size     int
}
type MavenDeclarations struct {
	Dependencies []string `json:"dependencies,omitempty"`
	Managed      []string `json:"managed_dependencies,omitempty"`
	Imports      []string `json:"bom_imports,omitempty"`
}

var mavenName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func readMaven(data []byte, blob string) *mavenPOM {
	var m mavenPOM
	dec := xml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m); err != nil {
		return nil
	}
	m.mavenID.trim()
	if m.Parent != nil {
		m.Parent.mavenID.trim()
	}
	m.ModelVersion = strings.TrimSpace(m.ModelVersion)
	if m.Artifact == "" || (m.ModelVersion != "4.0.0" && m.ModelVersion != "4.1.0") {
		return nil
	}
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
		switch t := token.(type) {
		case xml.Comment, xml.ProcInst:
		case xml.CharData:
			if len(bytes.TrimSpace(t)) != 0 {
				return nil
			}
		default:
			return nil
		}
	}
	m.blob = blob
	m.size = len(data)
	return &m
}

func (id *mavenID) trim() {
	id.Group = strings.TrimSpace(id.Group)
	id.Artifact = strings.TrimSpace(id.Artifact)
	id.Version = strings.TrimSpace(id.Version)
}

func (m *mavenPOM) identity() mavenID {
	id := m.mavenID
	if m.Parent != nil {
		if id.Group == "" {
			id.Group = m.Parent.Group
		}
		if id.Version == "" {
			id.Version = m.Parent.Version
		}
	}
	return id
}

func expandMaven(raw string, props, overrides map[string]string) string {
	value := strings.TrimSpace(raw)
	for range 32 {
		if len(value) > 4096 {
			return ""
		}
		start := strings.Index(value, "${")
		if start < 0 {
			return value
		}
		key, tail, ok := strings.Cut(value[start+2:], "}")
		if !ok {
			return ""
		}
		replacement, known := overrides[key]
		if !known {
			replacement, known = props[key]
		}
		if !known || strings.HasPrefix(key, "env.") || strings.HasPrefix(key, "settings.") || len(replacement) > 4096 {
			return ""
		}
		value = value[:start] + replacement + tail
	}
	return ""
}

func mavenProps(group mavenProperties, props map[string]string) {
	for _, e := range group.Entries {
		props[e.XMLName.Local] = strings.TrimSpace(e.Value)
	}
}

func (m *mavenPOM) localParent(file string, poms map[string]*mavenPOM) string {
	ref := "../pom.xml"
	if m.Parent.Relative != nil {
		ref = strings.TrimSpace(*m.Parent.Relative)
	}
	ref = strings.ReplaceAll(ref, "\\", "/")
	if ref == "" || path.IsAbs(ref) || strings.Contains(ref, ":") {
		return ""
	}
	candidate := path.Join(path.Dir(file), ref)
	if poms[candidate] == nil {
		candidate = path.Join(candidate, "pom.xml")
	}
	parent := poms[candidate]
	if parent == nil {
		return ""
	}
	parentID := parent.identity()
	if parentID != m.Parent.mavenID {
		return ""
	}
	return candidate
}

func addMavenDependencies(s mavenSection, props, overrides map[string]string, dst *MavenDeclarations, unresolved, budget *int) bool {
	appendGroup := func(deps []mavenDependency, managed bool) bool {
		for _, d := range deps {
			g, a := expandMaven(d.Group, props, overrides), expandMaven(d.Artifact, props, overrides)
			if !mavenName.MatchString(g) || !mavenName.MatchString(a) {
				(*unresolved)++
				continue
			}
			id := g + ":" + a
			if len(id) > *budget {
				return false
			}
			*budget -= len(id)
			switch {
			case managed && expandMaven(d.Scope, props, overrides) == "import" && expandMaven(d.Type, props, overrides) == "pom":
				dst.Imports = append(dst.Imports, id)
			case managed:
				dst.Managed = append(dst.Managed, id)
			default:
				dst.Dependencies = append(dst.Dependencies, id)
			}
		}
		return true
	}
	return appendGroup(s.Deps, false) && appendGroup(s.Managed, true)
}

func (d *MavenDeclarations) sortUnique() {
	for _, group := range []*[]string{&d.Dependencies, &d.Managed, &d.Imports} {
		slices.Sort(*group)
		*group = slices.Compact(*group)
	}
}

func (f *InventoryFile) resolveMaven(m *mavenPOM, poms map[string]*mavenPOM, budget *int) {
	chain := []*mavenPOM{m}
	seen := map[string]bool{f.Path: true}
	file := f.Path
	for cur := m; cur.Parent != nil; {
		file = cur.localParent(file, poms)
		if file == "" || seen[file] || len(chain) >= 64 {
			f.UnresolvedInputs++
			break
		}
		cur = poms[file]
		if cur.size > *budget {
			f.Problem = "analysis_budget"
			return
		}
		*budget -= cur.size
		seen[file] = true
		chain = append(chain, cur)
		f.ParentSources = append(f.ParentSources, ManifestSource{Path: file, Blob: cur.blob})
	}
	props := map[string]string{}
	for i := len(chain) - 1; i >= 0; i-- {
		mavenProps(chain[i].Properties, props)
	}
	id := m.identity()
	for _, prefix := range []string{"project.", "pom."} {
		props[prefix+"groupId"], props[prefix+"artifactId"], props[prefix+"version"] = id.Group, id.Artifact, id.Version
		if m.Parent != nil {
			props[prefix+"parent.groupId"], props[prefix+"parent.artifactId"], props[prefix+"parent.version"] = m.Parent.Group, m.Parent.Artifact, m.Parent.Version
		}
	}
	all := MavenDeclarations{}
	for _, source := range chain {
		if !addMavenDependencies(source.mavenSection, props, nil, &all, &f.UnresolvedInputs, budget) {
			f.Problem = "analysis_budget"
			return
		}
		for _, p := range source.Profiles {
			if len(p.Deps)+len(p.Managed) == 0 {
				continue
			}
			p.ID = strings.TrimSpace(p.ID)
			if len(p.ID) > 256 || !mavenName.MatchString(p.ID) {
				f.UnresolvedInputs++
				continue
			}
			if f.ProfileDependencies == nil {
				f.ProfileDependencies = map[string]MavenDeclarations{}
			}
			conditional := f.ProfileDependencies[p.ID]
			local := map[string]string{}
			mavenProps(p.Properties, local)
			if !addMavenDependencies(p.mavenSection, props, local, &conditional, &f.UnresolvedInputs, budget) {
				f.Problem = "analysis_budget"
				return
			}
			f.ProfileDependencies[p.ID] = conditional
		}
	}
	all.sortUnique()
	for id, declarations := range f.ProfileDependencies {
		declarations.sortUnique()
		f.ProfileDependencies[id] = declarations
	}
	f.Dependencies = all.Dependencies
	f.ManagedDependencies = all.Managed
	f.BOMImports = all.Imports
}
