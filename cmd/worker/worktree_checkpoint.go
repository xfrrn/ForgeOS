package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

// Hash the staged index and the tree the Worker would commit without staging anything in the real checkout.
func checkpointTrees(ctx context.Context, workspace string) (store.WorktreeTrees, error) {
	var trees store.WorktreeTrees
	if err := checkGitFilters(ctx, workspace); err != nil {
		return trees, err
	}
	indexPath, err := localrepo.Git(ctx, workspace, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return trees, err
	}
	source, err := os.Open(strings.TrimSpace(string(indexPath)))
	if err != nil {
		return trees, err
	}
	defer source.Close()
	dir, err := os.MkdirTemp("", "forgeos-checkpoint-")
	if err != nil {
		return trees, err
	}
	defer os.RemoveAll(dir)
	index := filepath.Join(dir, "index")
	dst, err := os.OpenFile(index, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return trees, err
	}
	_, err = io.Copy(dst, source)
	if err = errors.Join(err, dst.Close()); err != nil {
		return trees, err
	}
	run := func(args ...string) (string, error) {
		cmd := localrepo.GitCommand(ctx, workspace, args...)
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+index)
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	if trees.Index, err = run("write-tree"); err != nil {
		return trees, err
	}
	// A fresh index forces Git to hash tracked files even when their size and mtime match.
	if err = os.Remove(index); err != nil {
		return trees, err
	}
	if _, err = run("read-tree", trees.Index); err != nil {
		return trees, err
	}
	if _, err = run("add", "-A", "--", "."); err != nil {
		return trees, err
	}
	trees.Worktree, err = run("write-tree")
	return trees, err
}
