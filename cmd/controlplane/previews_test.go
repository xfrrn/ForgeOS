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

func TestPreviewReadsCommittedPublicFiles(t *testing.T) {
	_, repo, _ := issueTestRepo(t)
	git := func(args ...string) string {
		t.Helper()
		out, err := localrepo.Git(context.Background(), repo, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.Mkdir(filepath.Join(repo, "public"), 0700); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(repo, "public", "index.html")
	if err := os.WriteFile(index, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "public/index.html")
	blob := git("rev-parse", ":public/index.html")
	git("update-index", "--add", "--cacheinfo", "120000,"+blob+",public/link")
	git("-c", "user.name=QA", "-c", "user.email=qa@example.invalid", "commit", "-qm", "Static site")
	sha := git("rev-parse", "HEAD")
	if err := os.WriteFile(index, []byte("uncommitted"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := previewBlob(context.Background(), repo, sha, "public/index.html")
	if err != nil || string(data) != "candidate" {
		t.Fatalf("fixed asset = %q, %v", data, err)
	}
	for _, name := range []string{"public/link", "public/.env", "public/../README.md", "public\\index.html", "public/", "public/*", "public"} {
		if _, err := previewBlob(context.Background(), repo, sha, name); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("accepted path %q: %v", name, err)
		}
	}
}
