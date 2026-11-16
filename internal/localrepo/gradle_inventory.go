package localrepo

import (
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

var gradleAliasName = regexp.MustCompile(`^[a-z][a-zA-Z0-9_.-]+$`)
var gradleAliasSeparator = strings.NewReplacer("-", ".", "_", ".")

func gradleAlias(name string) string {
	if !gradleAliasName.MatchString(name) {
		return ""
	}
	return gradleAliasSeparator.Replace(name)
}

func catalogCoordinate(entry any, versions map[string]bool) string {
	var group, name string
	switch value := entry.(type) {
	case string:
		parts := strings.Split(value, ":")
		if len(parts) != 3 || strings.TrimSpace(parts[2]) == "" {
			return ""
		}
		group, name = parts[0], parts[1]
	case map[string]any:
		for key := range value {
			switch key {
			case "module", "group", "name", "version":
			default:
				return ""
			}
		}
		if version, ok := value["version"].(map[string]any); ok {
			if ref, exists := version["ref"]; exists {
				key, ok := ref.(string)
				if !ok || !versions[gradleAlias(key)] {
					return ""
				}
			}
		}
		if module, exists := value["module"]; exists {
			text, ok := module.(string)
			if !ok {
				return ""
			}
			group, name, _ = strings.Cut(text, ":")
		} else {
			group, _ = value["group"].(string)
			name, _ = value["name"].(string)
		}
	default:
		return ""
	}
	group, name = strings.TrimSpace(group), strings.TrimSpace(name)
	if !mavenName.MatchString(group) || !mavenName.MatchString(name) {
		return ""
	}
	return group + ":" + name
}

func (f *InventoryFile) readGradleCatalog(data []byte) bool {
	var doc struct {
		Versions  map[string]any
		Libraries map[string]any
		Bundles   map[string][]string
	}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return false
	}
	versions := make(map[string]bool, len(doc.Versions))
	for name := range doc.Versions {
		alias := gradleAlias(name)
		if alias == "" {
			return false
		}
		versions[alias] = true
	}
	libraries := make(map[string]string, len(doc.Libraries))
	for name, entry := range doc.Libraries {
		alias, coord := gradleAlias(name), catalogCoordinate(entry, versions)
		if alias == "" || coord == "" || libraries[alias] != "" {
			return false
		}
		libraries[alias] = coord
	}
	bundles := make(map[string][]string, len(doc.Bundles))
	for name, entries := range doc.Bundles {
		alias := gradleAlias(name)
		if _, duplicate := bundles[alias]; alias == "" || duplicate {
			return false
		}
		members := make([]string, 0, len(entries))
		for _, entry := range entries {
			key := gradleAlias(entry)
			if libraries[key] == "" {
				return false
			}
			members = append(members, key)
		}
		bundles[alias] = members
	}
	f.CatalogLibraries, f.CatalogBundles = libraries, bundles
	return true
}
