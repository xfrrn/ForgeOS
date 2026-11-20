package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSyncResultWorktreeAtManagedRoot(t *testing.T) {
	managed := t.TempDir()
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", managed)
	w := &worker{repoRoot: t.TempDir(), workspaceRoot: t.TempDir()}
	repo := filepath.Join(managed, "example")
	git(t, managed, "init", "--template=", repo)
	git(t, repo, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-qm", "bootstrap")
	workspace := filepath.Join(w.workspaceRoot, "execution-1")
	git(t, repo, "worktree", "add", "--detach", workspace, "HEAD")
	if err := w.syncResultWorktree(context.Background(), workspace); err != nil {
		t.Fatalf("synchronizing a managed worktree: %v", err)
	}
	t.Setenv("FORGEOS_MANAGED_REPO_ROOT", t.TempDir())
	if err := w.syncResultWorktree(context.Background(), workspace); err == nil {
		t.Fatal("synchronized metadata outside the configured repository roots")
	}
}
