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

func ReadPlan(ctx context.Context, repo, commit, level string) (Plan, error) {
	var plan Plan
	if !FullCommit(commit) || !ValidLevel(level) {
		return plan, errors.New("provide a full commit and a level V1 through V4")
	}
	out, err := localrepo.Git(ctx, repo, "cat-file", "-t", commit)
	if err != nil || string(out) != "commit\n" {
		return plan, errors.New("validation revision is not a commit")
	}
	entry, err := localrepo.Git(ctx, repo, "ls-tree", commit, "--", "validation.yaml")
	fields := strings.Fields(string(entry))
	if err != nil || len(fields) != 4 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !fullCommit.MatchString(fields[2]) || fields[3] != "validation.yaml" {
		return plan, errors.New("commit requires a regular validation.yaml file")
	}
	raw, large, err := localrepo.LimitedGit(ctx, repo, schemas.MaxValidationBytes, "cat-file", "blob", fields[2])
	if err != nil || large {
		return plan, errors.New("validation.yaml too large or unreadable")
	}
	cfg, err := schemas.ParseValidation(raw)
	if err != nil {
		return plan, fmt.Errorf("invalid validation.yaml: %w", err)
	}
	plan = Plan{Commit: commit, ConfigBlob: fields[2], Version: cfg.Version, Level: level, Checks: []PlannedCheck{}, Missing: []string{}}
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
