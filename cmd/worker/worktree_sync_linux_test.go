package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncResultWorktree(t *testing.T) {
	w, job, head := fixture(t)
	ctx := context.Background()
	if result := w.execute(ctx, job); result.Commit != head {
		t.Fatal(result)
	}
	if err := w.syncResultWorktree(ctx, job.Workspace); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(w.workspaceRoot, "another-workspace")
	if err := os.Symlink(job.Workspace, link); err != nil {
		t.Fatal(err)
	}
	if err := w.syncResultWorktree(ctx, link); err == nil {
		t.Fatal("followed a replaced workspace")
	}
	w.repoRoot = t.TempDir()
	if err := w.syncResultWorktree(ctx, job.Workspace); err == nil {
		t.Fatal("accepted Git metadata outside the repository root")
	}
}
