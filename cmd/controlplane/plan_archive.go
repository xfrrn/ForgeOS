package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func archivePlan(ctx context.Context, repo, base string, rev store.PlanRevision) (string, error) {
	if !gitCommit.MatchString(base) || rev.TaskID <= 0 || rev.Revision <= 0 {
		return "", store.ErrConflict
	}
	content, err := json.MarshalIndent(rev.Plan, "", "  ")
	if err != nil {
		return "", err
	}
	// A private index keeps the owner's staged changes and checkout out of the archive.
	dir, err := os.MkdirTemp("", "forgeos-plan-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	run := func(input []byte, args ...string) (string, error) {
		cmd := localrepo.GitCommand(ctx, repo, args...)
		date := rev.At.UTC().Format("2006-01-02T15:04:05Z")
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+filepath.Join(dir, "index"), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		cmd.Stdin = bytes.NewReader(input)
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	if _, err := run(nil, "read-tree", base); err != nil {
		return "", errors.New("plan base commit is unavailable")
	}
	blob, err := run(append(content, '\n'), "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf(".forgeos/plans/goal-%d.json", rev.TaskID)
	if _, err = run(nil, "update-index", "--add", "--cacheinfo", "100644", blob, path); err != nil {
		return "", err
	}
	tree, err := run(nil, "write-tree")
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("goal-%d/r%d", rev.TaskID, rev.Revision)
	sha, err := run(nil, "-c", "user.name=ForgeOS", "-c", "user.email=plans@forgeos.local",
		"commit-tree", "--no-gpg-sign", tree, "-p", base, "-m", "Record ForgeOS plan "+name)
	if err != nil {
		return "", err
	}
	if rev.GitCommit != "" && rev.GitCommit != sha {
		return "", store.ErrConflict
	}
	ref := "refs/forgeos/plans/" + name
	if _, err = run(nil, "update-ref", "--no-deref", ref, sha, strings.Repeat("0", len(sha))); err != nil {
		existing, readErr := run(nil, "rev-parse", "--verify", ref)
		if readErr != nil {
			return "", err
		}
		if existing != sha {
			return "", store.ErrConflict
		}
	}
	return sha, nil
}
