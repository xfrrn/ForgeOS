// Package validation loads project checks from fixed Git objects.
package validation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/schemas"
)

var fullCommit = regexp.MustCompile("^(?:[a-f0-9]{40}|[a-f0-9]{64})$")
var levels = []struct {
	level, profile string
	kinds          []string
}{
	{"V1", "fast", []string{"static"}},
	{"V2", "standard", []string{"unit", "build"}},
	{"V3", "integration", []string{"integration"}},
	{"V4", "critical", []string{"security", "regression"}},
}

type PlannedCheck struct {
	Name string `json:"name"`
	schemas.Check
}

type Plan struct {
	Commit     string         `json:"commit"`
	ConfigBlob string         `json:"config_blob"`
	Version    int            `json:"version"`
	Level      string         `json:"level"`
	Profile    string         `json:"profile"`
	Checks     []PlannedCheck `json:"checks"`
	Missing    []string       `json:"missing"`
	Ready      bool           `json:"ready"`
}

func FullCommit(sha string) bool { return fullCommit.MatchString(sha) }
func ValidLevel(level string) bool {
	return level == "V1" || level == "V2" || level == "V3" || level == "V4"
}

// ReadConfigFile returns the committed regular file and its blob ID, or an empty ID when absent.
func ReadConfigFile(ctx context.Context, repo, commit, name string) (string, []byte, error) {
	if !FullCommit(commit) {
		return "", nil, errors.New("provide a full commit hash")
	}
	out, err := localrepo.Git(ctx, repo, "cat-file", "-t", commit)
	if err != nil || string(out) != "commit\n" {
		return "", nil, errors.New("configuration revision is not a commit")
	}
	entry, err := localrepo.Git(ctx, repo, "ls-tree", commit, "--", name)
	if err == nil && len(entry) == 0 {
		return "", nil, nil
	}
	fields := strings.Fields(string(entry))
	if err != nil || len(fields) != 4 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !fullCommit.MatchString(fields[2]) || fields[3] != name {
		return "", nil, fmt.Errorf("commit requires a regular %s file", name)
	}
	raw, large, err := localrepo.LimitedGit(ctx, repo, schemas.MaxValidationBytes, "cat-file", "blob", fields[2])
	if err != nil || large {
		return "", nil, fmt.Errorf("%s too large or unreadable", name)
	}
	return fields[2], raw, nil
}

func ReadPlan(ctx context.Context, repo, commit, level string) (Plan, error) {
	var plan Plan
	if !ValidLevel(level) {
		return plan, errors.New("provide a level V1 through V4")
	}
	blob, raw, err := ReadConfigFile(ctx, repo, commit, "validation.yaml")
	if err != nil {
		return plan, err
	}
	if blob == "" {
		return plan, errors.New("commit requires a regular validation.yaml file")
	}
	cfg, err := schemas.ParseValidation(raw)
	if err != nil {
		return plan, fmt.Errorf("invalid validation.yaml: %w", err)
	}
	plan = Plan{Commit: commit, ConfigBlob: blob, Version: cfg.Version, Level: level, Checks: []PlannedCheck{}, Missing: []string{}}
	seen, kinds := map[string]bool{}, map[string]bool{}
	var required []string
	for _, l := range levels {
		required = append(required, l.kinds...)
		profile, exists := cfg.Profiles[l.profile]
		for _, id := range profile.Checks {
			if seen[id] {
				continue
			}
			seen[id] = true
			c := cfg.Checks[id]
			plan.Checks = append(plan.Checks, PlannedCheck{id, c})
			if c.Required == nil || *c.Required {
				kinds[c.Type] = true
			}
		}
		if l.level == level {
			plan.Profile = l.profile
			if !exists {
				plan.Missing = append(plan.Missing, "profile:"+l.profile)
			}
			break
		}
	}
	for _, kind := range required {
		if !kinds[kind] {
			plan.Missing = append(plan.Missing, "check:"+kind)
		}
	}
	plan.Ready = len(plan.Missing) == 0
	return plan, nil
}
