package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"forgeos/internal/localrepo"
	"golang.org/x/sys/unix"
)

// SQLite's FULL commit does not flush the worktree or Git metadata on other mounts.
func (w *worker) syncResultWorktree(ctx context.Context, workspace string) error {
	dir, err := filepath.EvalSymlinks(workspace)
	if err != nil || dir != workspace || filepath.Dir(dir) != w.workspaceRoot {
		return errors.New("execution worktree is unavailable or outside the workspace root")
	}
	out, err := localrepo.Git(ctx, dir, "rev-parse", "--path-format=absolute", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		return err
	}
	metadata := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(metadata) != 2 {
		return errors.New("invalid Git metadata paths")
	}
	dirs := []string{dir}
	for _, path := range metadata {
		real, err := filepath.EvalSymlinks(path)
		if err != nil || !filepath.IsAbs(real) {
			return errors.New("Git metadata is unavailable")
		}
		rel, err := filepath.Rel(w.repoRoot, real)
		if err != nil || !filepath.IsLocal(rel) {
			return errors.New("Git metadata outside repository root")
		}
		dirs = append(dirs, real)
	}
	devices := make(map[uint64]bool)
	for _, path := range dirs {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		var st unix.Stat_t
		err = unix.Fstat(fd, &st)
		if err == nil && !devices[uint64(st.Dev)] {
			err = unix.Syncfs(fd)
			devices[uint64(st.Dev)] = true
		}
		unix.Close(fd)
		if err != nil {
			return err
		}
	}
	return nil
}
