package validation

import (
	"bytes"
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"forgeos/internal/localrepo"
	"forgeos/schemas"
)

type Policy struct {
	Minimum    string `json:"minimum,omitempty"`
	TaskType   string `json:"task_type,omitempty"`
	Target     string `json:"target,omitempty"`
	Management string `json:"management_minimum,omitempty"`
}

func (p Policy) Valid() bool {
	return (p.Minimum == "" || ValidLevel(p.Minimum)) && (p.Management == "" || ValidLevel(p.Management)) &&
		(p.TaskType == "" || p.TaskType == "change" || p.TaskType == "documentation" || p.TaskType == "integration" || p.TaskType == "security") &&
		(p.Target == "" || p.Target == "local" || p.Target == "pull_request" || p.Target == "staging")
}

type Match struct {
	Component string                    `json:"component"`
	Count     int                       `json:"path_count"`
	Example   string                    `json:"example_path"`
	Playbooks []string                  `json:"playbooks"`
	Manual    *schemas.ManualAcceptance `json:"manual_acceptance,omitempty"`
	Expected  []string                  `json:"expected_behavior,omitempty"`
}
type Reason struct {
	Source   string `json:"source"`
	Level    string `json:"level"`
	Evidence string `json:"evidence"`
}
type Decision struct {
	Base      string   `json:"base_commit"`
	Commit    string   `json:"commit"`
	RulesBlob string   `json:"rules_blob,omitempty"`
	Policy    Policy   `json:"policy"`
	Level     string   `json:"level"`
	Changed   int      `json:"changed_paths"`
	Matches   []Match  `json:"matches"`
	Reasons   []Reason `json:"reasons"`
}

func (d *Decision) floor(source, level, evidence string) {
	if level == "" {
		return
	}
	d.Reasons = append(d.Reasons, Reason{source, level, evidence})
	d.Level = max(d.Level, level)
}

func changeRisk(file string) string {
	parts := strings.Split(strings.ToLower(file), "/")
	for _, s := range parts {
		switch s {
		case "auth", "authentication", "authorization", "payment", "payments", "secrets", "migrations":
			return "V4"
		}
	}
	if file == "acceptance.yaml" || file == "validation.yaml" {
		return "V4"
	}
	for _, s := range parts {
		switch s {
		case "integration", "e2e", ".github", "deploy":
			return "V3"
		}
	}
	switch strings.ToLower(path.Ext(file)) {
	case ".md", ".rst", ".adoc", ".txt":
		return "V1"
	}
	return "V2"
}

func Assess(ctx context.Context, repo, base, commit string, p Policy) (Decision, error) {
	d := Decision{Base: base, Commit: commit, Policy: p, Matches: []Match{}, Reasons: []Reason{}}
	if !p.Valid() || !FullCommit(base) || !FullCommit(commit) || base == commit {
		return d, errors.New("invalid risk inputs or fixed revisions")
	}
	out, err := localrepo.Git(ctx, repo, "cat-file", "-t", commit)
	if err != nil || string(out) != "commit\n" {
		return d, errors.New("candidate must be a commit")
	}
	if _, err := localrepo.Git(ctx, repo, "merge-base", "--is-ancestor", base, commit); err != nil {
		return d, errors.New("candidate must descend from the selected base")
	}
	blob, raw, err := ReadConfigFile(ctx, repo, base, "acceptance.yaml")
	if err != nil {
		return d, err
	}
	var cfg schemas.Acceptance
	if blob != "" {
		cfg, err = schemas.ParseAcceptance(raw)
		if err != nil {
			return d, err
		}
		d.RulesBlob = blob
	}
	names, large, err := localrepo.LimitedGit(ctx, repo, 1<<20, "diff", "--no-renames", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "--name-only", "-z", base, commit, "--")
	if err != nil || large || len(names) == 0 || !utf8.Valid(names) || names[len(names)-1] != 0 {
		return d, errors.New("empty, unreadable or oversized changed path list")
	}
	files := strings.Split(string(bytes.TrimSuffix(names, []byte{0})), "\x00")
	if len(files) > 4096 {
		return d, errors.New("candidate exceeds 4096 changed paths")
	}
	d.Changed = len(files)
	highest, pathEvidence := "", ""
	for _, file := range files {
		if l := changeRisk(file); l > highest {
			highest, pathEvidence = l, file
		}
	}
	d.floor("diff", highest, pathEvidence)
	d.floor("project", cfg.Minimum, "acceptance.yaml")
	d.floor("owner", p.Minimum, "requested minimum")
	switch p.TaskType {
	case "change":
		d.floor("task", "V2", p.TaskType)
	case "integration":
		d.floor("task", "V3", p.TaskType)
	case "security":
		d.floor("task", "V4", p.TaskType)
	case "documentation":
		d.floor("task", "V1", p.TaskType)
	}
	switch p.Target {
	case "pull_request":
		d.floor("target", "V2", p.Target)
	case "staging":
		d.floor("target", "V3", p.Target)
	case "local":
		d.floor("target", "V1", p.Target)
	}
	var ids []string
	for id := range cfg.Components {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := cfg.Components[id]
		m := Match{Component: id, Playbooks: c.Playbooks, Manual: c.Manual, Expected: c.Expected}
		for _, file := range files {
			if c.Matches(file) {
				m.Count++
				if m.Example == "" {
					m.Example = file
				}
			}
		}
		if m.Count > 0 {
			d.Matches = append(d.Matches, m)
			d.floor("component", c.Minimum, id)
		}
	}
	d.floor("management", p.Management, "requested minimum")
	return d, nil
}
