package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"forgeos/internal/localrepo"
)

func TestProjectGitFastForward(t *testing.T) {
	_, repo, base := issueTestRepo(t)
	ctx := context.Background()
	git := func(args ...string) {
		t.Helper()
		if _, err := localrepo.Git(ctx, repo, args...); err != nil {
			t.Fatalf("git %s: %v", args[0], err)
		}
	}
	read := func() projectCommit {
		t.Helper()
		value, err := readProjectCommit(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	start := read()
	if start.SHA != base || start.Branch == "" || start.Subject != "base" {
		t.Fatalf("wrong initial commit: %+v", start)
	}
	upstream, err := findProjectUpstream(ctx, repo, start.Branch)
	if err != nil || upstream.name != "origin" || upstream.ref != "refs/heads/"+start.Branch {
		t.Fatalf("wrong upstream: %+v %v", upstream, err)
	}
	git("switch", "-c", "incoming")
	git("-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "--allow-empty", "-qm", "incoming")
	incoming := read()
	git("switch", start.Branch)
	if err := os.WriteFile(filepath.Join(repo, "draft.txt"), []byte("local draft"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := fastForwardProject(ctx, repo, start, incoming.SHA); err == nil || read().SHA != start.SHA {
		t.Fatal("pull must preserve a dirty checkout")
	}
	if err := os.Remove(filepath.Join(repo, "draft.txt")); err != nil {
		t.Fatal(err)
	}
	if updated, err := fastForwardProject(ctx, repo, start, incoming.SHA); err != nil || !updated || read().SHA != incoming.SHA {
		t.Fatalf("fast-forward was not applied: updated=%v err=%v", updated, err)
	}
	if updated, err := fastForwardProject(ctx, repo, read(), start.SHA); err != nil || updated {
		t.Fatalf("pull must preserve a local branch ahead of the remote: updated=%v err=%v", updated, err)
	}
	git("switch", "-c", "diverged", base)
	git("-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "--allow-empty", "-qm", "different")
	diverged := read()
	if _, err := fastForwardProject(ctx, repo, diverged, incoming.SHA); err == nil || read().SHA != diverged.SHA {
		t.Fatal("pull must preserve a diverged branch")
	}
}

func TestProjectGitNetworkPull(t *testing.T) {
	if os.Getenv("FORGEOS_TEST_GIT_CLONE") != "1" {
		t.Skip("set FORGEOS_TEST_GIT_CLONE=1 to exercise GitHub transport")
	}
	_, repo, base := issueTestRepo(t)
	ctx := context.Background()
	remote, err := parseCloneRemote("https://github.com/octocat/Hello-World.git", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target, err := readProjectRemote(ctx, dir, projectUpstream{remote: remote, name: "origin", ref: "refs/heads/master"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fetchProjectCommit(ctx, repo, dir, remote, target.SHA, base); err != nil {
		t.Fatal(err)
	}
	if _, err := localrepo.Git(ctx, repo, "switch", "-c", "tracking", target.SHA+"^"); err != nil {
		t.Fatal(err)
	}
	before, err := readProjectCommit(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if updated, err := fastForwardProject(ctx, repo, before, target.SHA); err != nil || !updated {
		t.Fatalf("fetched remote commit did not fast-forward the branch: %v", err)
	}
}
