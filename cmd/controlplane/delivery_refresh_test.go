package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func TestRefreshCommitPreservesCheckoutAndApprovedPatch(t *testing.T) {
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
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		git("add", "--", name)
	}
	commit := func() string {
		git("-c", "user.name=test", "-c", "user.email=test@example.org", "commit", "-m", "fixture")
		return git("rev-parse", "HEAD")
	}
	write("guide.md", "accepted\n")
	previous := commit()
	git("checkout", "--detach", base)
	write("other.md", "new main\n")
	target := commit()
	write("pending.txt", "local staged change")
	staged := git("write-tree")
	sha, err := refreshCommit(ctx, repo, base, previous, target)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := refreshCommit(ctx, repo, base, previous, target)
	if err != nil || replay != sha || git("rev-parse", "HEAD") != target || git("write-tree") != staged || git("rev-parse", sha+"^") != target ||
		git("diff", "--name-only", target, sha) != "guide.md" || git("show", sha+":guide.md") != "accepted" {
		t.Fatal(sha, replay, err)
	}
	write("guide.md", "conflicting target\n")
	conflict := commit()
	if _, err = refreshCommit(ctx, repo, base, previous, conflict); !errors.Is(err, store.ErrConflict) {
		t.Fatal("conflicting patch accepted", err)
	}
}
