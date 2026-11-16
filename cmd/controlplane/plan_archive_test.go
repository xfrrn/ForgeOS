package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func TestPlanArchivePreservesIndexAndRef(t *testing.T) {
	_, repo, base := issueTestRepo(t)
	ctx := context.Background()
	git := func(args ...string) string {
		t.Helper()
		out, err := localrepo.Git(ctx, repo, args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.WriteFile(filepath.Join(repo, "pending.txt"), []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "pending.txt")
	index := git("write-tree")
	r := store.PlanRevision{TaskID: 5, Revision: 2, At: time.Now(), Plan: store.Plan{Objective: "Plan evidence"}}
	sha, err := archivePlan(ctx, repo, base, r)
	if err != nil {
		t.Fatal(err)
	}
	if got := git("diff", "--name-only", base, sha); got != ".forgeos/plans/goal-5.json" {
		t.Fatal(got)
	}
	if git("rev-parse", "HEAD") != base || git("write-tree") != index {
		t.Fatal("changed owner checkout or index")
	}
	if again, err := archivePlan(ctx, repo, base, r); err != nil || again != sha {
		t.Fatal("non-idempotent archive", again, err)
	}
	ref := "refs/forgeos/plans/goal-5/r2"
	git("update-ref", ref, base, sha)
	if _, err := archivePlan(ctx, repo, base, r); !errors.Is(err, store.ErrConflict) || git("rev-parse", ref) != base {
		t.Fatal("overwrote a conflicting reference", err)
	}
}
