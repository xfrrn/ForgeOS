package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCappedLogContinuesDraining(t *testing.T) {
	var buf bytes.Buffer
	log := cappedLog{dest: &buf, remaining: 3}
	for _, input := range []string{"ab", "cd", "ef"} {
		n, err := log.Write([]byte(input))
		if err != nil || n != len(input) {
			t.Fatalf("write returned %d, %v", n, err)
		}
	}
	if buf.String() != "abc\n[remaining CI output omitted]\n" {
		t.Fatal("unexpected bounded log contents")
	}
}

func validationCommit(t *testing.T, repo, name, contents string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "--", name)
	git(t, repo, "-c", "user.name=CI", "-c", "user.email=ci@localhost", "commit", "-qm", "validation fixture")
	return git(t, repo, "rev-parse", "HEAD")
}

func TestDocsValidationUsesCommittedContent(t *testing.T) {
	_, job, base := fixture(t)
	git(t, job.RepoPath, "config", "core.autocrlf", "false")
	candidate := validationCommit(t, job.RepoPath, "README.md", "A note.\r\n")
	ctx := context.Background()
	result, err := validateCandidate(ctx, job.RepoPath, base, candidate, "docs", nil)
	if err != nil || result.ExitCode != 0 || result.Commit != candidate {
		t.Fatalf("unexpected docs result: %+v, %v", result, err)
	}
	other := validationCommit(t, job.RepoPath, "main.go", "package main\n")
	if _, err := validateCandidate(ctx, job.RepoPath, base, other, "docs", nil); err == nil {
		t.Fatal("code change passed the docs profile")
	}
	if _, err := validateCandidate(ctx, job.RepoPath, base, candidate, "docs", nil); err == nil {
		t.Fatal("stale candidate passed")
	}
	bad := validationCommit(t, job.RepoPath, "guide.md", "trailing space \r\n")
	if _, err := validateCandidate(ctx, job.RepoPath, other, bad, "docs", nil); err == nil {
		t.Fatal("whitespace error passed")
	}
}

func TestGoValidationInLocalContainer(t *testing.T) {
	if os.Getenv("FORGEOS_TEST_DOCKER") != "1" {
		t.Skip("set FORGEOS_TEST_DOCKER=1 to exercise the local offline CI image")
	}
	_, job, base := fixture(t)
	repo := job.RepoPath
	validationCommit(t, repo, "go.mod", "module example\n\ngo 1.23\n")
	good := validationCommit(t, repo, "main_test.go", "package example\nimport \"testing\"\nfunc TestWorks(t *testing.T) {}\n")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := validateCandidate(ctx, repo, base, good, "go-test", nil)
	if err != nil || result.ExitCode != 0 || result.Commit != good {
		t.Fatalf("Go validation: %+v, %v", result, err)
	}
	// Each actual execution has its own workspace and log.
	bad := validationCommit(t, repo, "main_test.go", "package example\nimport \"testing\"\nfunc TestWorks(t *testing.T) {t.Fatal(\"regression\")}\n")
	worktree := filepath.Join(t.TempDir(), "failed-run")
	git(t, repo, "worktree", "add", "--detach", worktree, bad)
	result, err = validateCandidate(ctx, worktree, good, bad, "go-test", nil)
	if err == nil || result.ExitCode == 0 || result.Commit != bad {
		t.Fatalf("failing Go test was not recorded: %+v, %v", result, err)
	}
	output, err := os.ReadFile(worktree + ".go-test.log")
	if err != nil || !bytes.Contains(output, []byte("regression")) {
		t.Fatal("missing failing test diagnostic in the local CI log")
	}
}
