package schemas

import (
	_ "embed"
	"encoding/json"
	"errors"
	"io/fs"
	"regexp"
	"strings"
)

//go:embed acceptance.schema.json
var acceptanceJSON []byte
var acceptanceSchema = cachedSchema("urn:forgeos:acceptance:v1", acceptanceJSON)

type Acceptance struct {
	Version    int                  `json:"version"`
	Minimum    string               `json:"minimum_validation,omitempty"`
	Components map[string]Component `json:"components"`
}
type Component struct {
	Paths     []string          `json:"paths"`
	Playbooks []string          `json:"playbooks"`
	Minimum   string            `json:"minimum_validation"`
	Manual    *ManualAcceptance `json:"manual_acceptance,omitempty"`
	Expected  []string          `json:"expected_behavior,omitempty"`
	patterns  []*regexp.Regexp
}

type ManualAcceptance struct {
	Required    json.RawMessage `json:"required"`
	Environment string          `json:"environment,omitempty"`
}

var globWildcards = strings.NewReplacer(`\*`, "[^/]*", `\?`, "[^/]")

func ParseAcceptance(raw []byte) (Acceptance, error) {
	var cfg Acceptance
	if err := decodeConfig(raw, acceptanceSchema, &cfg); err != nil {
		return cfg, err
	}
	for id, c := range cfg.Components {
		for _, pattern := range c.Paths {
			if pattern == "." || !fs.ValidPath(pattern) || strings.ContainsAny(pattern, "\\:\x00\r\n") {
				return cfg, errors.New("invalid project-relative component path")
			}
			var expr strings.Builder
			expr.WriteString("^")
			parts := strings.Split(pattern, "/")
			for i, part := range parts {
				if part == "**" {
					if i == len(parts)-1 {
						expr.WriteString("(?s:.*)")
					} else {
						expr.WriteString("(?:[^/]+/)*")
					}
					continue
				}
				if strings.Contains(part, "**") {
					return cfg, errors.New("recursive wildcard must occupy a whole path segment")
				}
				part = regexp.QuoteMeta(part)
				expr.WriteString(globWildcards.Replace(part))
				if i < len(parts)-1 {
					expr.WriteString("/")
				}
			}
			expr.WriteString("$")
			re, err := regexp.Compile(expr.String())
			if err != nil {
				return cfg, err
			}
			c.patterns = append(c.patterns, re)
		}
		cfg.Components[id] = c
	}
	return cfg, nil
}

func (c Component) Matches(path string) bool {
	for _, re := range c.patterns {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
