package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"forgeos/internal/store"
)

func TestCheckpointTreesDistinguishStagedAndUnstagedChanges(t *testing.T) {
	_, job, _ := fixture(t)
	repo := job.RepoPath
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "ignored\n")
	write("tracked", "old")
	git(t, repo, "add", ".")
	index := git(t, repo, "rev-parse", "--path-format=absolute", "--git-path", "index")
	readIndex := func() []byte {
		t.Helper()
		data, err := os.ReadFile(index)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	snapshot := func() store.WorktreeTrees {
		t.Helper()
		before := readIndex()
		trees, err := checkpointTrees(context.Background(), repo)
		if err != nil || !bytes.Equal(before, readIndex()) {
			t.Fatalf("snapshot changed index or failed: %v", err)
		}
		return trees
	}
	first := snapshot()
	if first.Index != first.Worktree || first.Index != git(t, repo, "write-tree") {
		t.Fatal(first)
	}
	write("ignored", "ignored bytes")
	if snapshot() != first {
		t.Fatal("included ignored file")
	}
	path := filepath.Join(repo, "tracked")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	write("tracked", "new")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	edited := snapshot()
	if edited.Index != first.Index || edited.Worktree == first.Worktree {
		t.Fatal("missed same-sized edit", edited)
	}
	git(t, repo, "add", "--renormalize", "--", "tracked")
	staged := snapshot()
	if staged.Index != edited.Worktree || staged.Worktree != edited.Worktree {
		t.Fatal("missed staged edit", staged)
	}
	write("untracked", "new file")
	withNew := snapshot()
	if withNew.Index != staged.Index || withNew.Worktree == staged.Worktree {
		t.Fatal("missed new file", withNew)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	deleted := snapshot()
	if deleted.Index != staged.Index || deleted.Worktree == withNew.Worktree {
		t.Fatal("missed deletion", deleted)
	}
}
