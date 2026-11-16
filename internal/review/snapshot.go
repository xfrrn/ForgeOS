// Package review ties code review input and evidence to immutable Git objects.
package review

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"forgeos/internal/localrepo"
)

var revision = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)

type File struct {
	Path       string   `json:"path"`
	BeforeMode string   `json:"before_mode"`
	AfterMode  string   `json:"after_mode"`
	Before     []string `json:"before"`
	After      []string `json:"after"`
}

type Snapshot struct {
	Base   string `json:"base_commit"`
	Commit string `json:"commit"`
	Files  []File `json:"files"`
}

func Read(ctx context.Context, repo, base, commit string) (Snapshot, error) {
	result := Snapshot{Base: base, Commit: commit}
	if !revision.MatchString(base) || !revision.MatchString(commit) || base == commit {
		return result, errors.New("provide two distinct full commit hashes")
	}
	for _, sha := range []string{base, commit} {
		out, err := localrepo.Git(ctx, repo, "cat-file", "-t", sha)
		if err != nil || string(out) != "commit\n" {
			return result, errors.New("review revision is not a commit")
		}
	}
	if _, err := localrepo.Git(ctx, repo, "merge-base", "--is-ancestor", base, commit); err != nil {
		return result, errors.New("candidate must descend from the supplied base")
	}
	raw, large, err := localrepo.LimitedGit(ctx, repo, 64<<10, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--ignore-submodules=none", "--raw", "--no-abbrev", "-z", base, commit, "--")
	if err != nil || large || len(raw) == 0 {
		return result, errors.New("empty, oversized or unreadable review change list")
	}
	parts := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	if len(parts)%2 != 0 || len(parts) > 256 {
		return result, errors.New("invalid or oversized changed file set")
	}
	remaining := 192 << 10
	blob := func(mode, id string) ([]string, error) {
		if mode == "000000" {
			return nil, nil
		}
		if mode != "100644" && mode != "100755" {
			return nil, errors.New("non-regular file requires manual review")
		}
		body, truncated, err := localrepo.LimitedGit(ctx, repo, remaining, "cat-file", "blob", id)
		if err != nil || truncated || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
			return nil, errors.New("binary or oversized review input requires manual review")
		}
		remaining -= len(body)
		if len(body) == 0 {
			return []string{}, nil
		}
		content := strings.ReplaceAll(string(body), "\r\n", "\n")
		return strings.Split(strings.TrimSuffix(content, "\n"), "\n"), nil
	}
	for i := 0; i < len(parts); i += 2 {
		fields := strings.Fields(strings.TrimPrefix(string(parts[i]), ":"))
		if len(fields) != 5 || !revision.MatchString(fields[2]) || !revision.MatchString(fields[3]) || !utf8.Valid(parts[i+1]) {
			return result, errors.New("invalid Git change record")
		}
		f := File{Path: string(parts[i+1]), BeforeMode: fields[0], AfterMode: fields[1]}
		f.Before, err = blob(fields[0], fields[2])
		if err != nil {
			return result, err
		}
		f.After, err = blob(fields[1], fields[3])
		if err != nil {
			return result, err
		}
		result.Files = append(result.Files, f)
	}
	return result, nil
}
