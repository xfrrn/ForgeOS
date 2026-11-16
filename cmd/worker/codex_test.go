package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/store"
)

// The test executable doubles as a local JSONL CLI, on both Windows and Unix.
func TestMain(m *testing.M) {
	if len(os.Args) <= 1 || os.Args[1] != "-a" {
		os.Exit(m.Run())
	}
	body, err := io.ReadAll(os.Stdin)
	args := strings.Join(os.Args[1:], " ")
	if err != nil || !strings.Contains(args, "-a never exec") || !strings.Contains(args, "--sandbox workspace-write") ||
		!strings.Contains(args, "--ignore-user-config") || os.Getenv("FORGEOS_WORKER_TOKEN") != "" || os.Getenv("GH_TOKEN") != "" {
		os.Exit(2)
	}
	if strings.Contains(string(body), "INVALID_SESSION") {
		fmt.Printf("{\"type\":\"thread.started\",\"thread_id\":\"%s\"}\n", strings.Repeat("x", 300))
		os.Exit(0)
	}
	session := "test-session"
	if strings.Contains(string(body), `"acceptance_feedback":`) {
		session = "test-review-session"
	}
	fmt.Printf("{\"type\":\"thread.started\",\"thread_id\":\"%s\"}\n", session)
	switch {
	case strings.Contains(string(body), "SIMULATED_FAILURE"):
		fmt.Println(`{"type":"turn.failed"}`)
		os.Exit(1)
	case strings.Contains(string(body), "SIMULATED_NO_CHANGE"):
	case strings.Contains(string(body), "APPEND_EXAMPLE"):
		current, err := os.ReadFile("README.md")
		if err != nil || string(current) != "An implemented change.\n" || !strings.Contains(string(body), `"title":"Change README"`) {
			os.Exit(4)
		}
		if err := os.WriteFile("README.md", append(current, []byte("Example: start here.\n")...), 0600); err != nil {
			os.Exit(3)
		}
	default:
		if err := os.WriteFile("README.md", []byte("An implemented change.\n"), 0600); err != nil {
			os.Exit(3)
		}
	}
	fmt.Println(`{"type":"turn.completed"}`)
	os.Exit(0)
}

func TestIssueChanges(t *testing.T) {
	t.Setenv("FORGEOS_WORKER_TOKEN", "worker-test-secret")
	t.Setenv("GH_TOKEN", "github-test-secret")
	for _, title := range []string{"Change README", "SIMULATED_FAILURE", "SIMULATED_NO_CHANGE", "INVALID_SESSION"} {
		t.Run(title, func(t *testing.T) {
			w, job, base := fixture(t)
			w.codex, _ = os.Executable()
			job.Kind = "codex_issue"
			job.Spec = &store.IssueSpec{Issue: store.Issue{Number: 42, Title: title}, BaseCommit: base, Validation: "docs"}
			res := w.execute(context.Background(), job)
			if title == "INVALID_SESSION" {
				if res.ExitCode == 0 || res.SessionID != "" {
					t.Fatalf("invalid session ID persisted: %+v", res)
				}
				return
			}
			if res.BaseCommit != base || res.SessionID != "test-session" {
				t.Fatalf("missing issue execution evidence: %+v", res)
			}
			if title != "Change README" {
				if res.ExitCode == 0 || res.Commit != "" || res.Validation != nil {
					t.Fatalf("invalid failed or empty result: %+v", res)
				}
				return
			}
			if res.ExitCode != 0 || res.Error != "" || res.Commit == "" || res.Commit == base ||
				res.Validation == nil || res.Validation.ExitCode != 0 || res.Validation.Commit != res.Commit {
				t.Fatalf("invalid validated candidate: %+v", res)
			}
			if git(t, job.Workspace, "rev-parse", "HEAD^") != base || git(t, job.Workspace, "rev-parse", "HEAD") != res.Commit {
				t.Fatal("candidate must be exactly one local commit on the pinned base")
			}
			if _, err := os.Stat(filepath.Join(job.RepoPath, "README.md")); !os.IsNotExist(err) || git(t, job.RepoPath, "rev-parse", "HEAD") != base {
				t.Fatal("the registered repository checkout was modified")
			}
		})
	}
}

func TestCandidateKeepsPinnedParentAfterAgentCommit(t *testing.T) {
	_, job, base := fixture(t)
	ctx := context.Background()
	identity, err := checkedWorktree(ctx, job.RepoPath, job.Workspace, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job.Workspace, "README.md"), []byte("updated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, job.Workspace, "add", "--", "README.md")
	git(t, job.Workspace, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "agent commit")
	tree := git(t, job.Workspace, "rev-parse", "HEAD^{tree}")
	candidate, err := commitChanges(ctx, job.Workspace, base, base, 7, identity)
	if err != nil || git(t, job.Workspace, "rev-parse", candidate+"^") != base || git(t, job.Workspace, "rev-parse", candidate+"^{tree}") != tree {
		t.Fatalf("unexpected candidate after an agent commit: %s %v", candidate, err)
	}
}

func TestIssueReworkRetainsRejectedWorkspace(t *testing.T) {
	w, first, base := fixture(t)
	w.codex, _ = os.Executable()
	first.Kind = "codex_issue"
	first.Spec = &store.IssueSpec{Issue: store.Issue{Number: 7, Title: "Change README"}, BaseCommit: base, Validation: "docs"}
	ctx := context.Background()
	rejected := w.execute(ctx, first)
	if rejected.ExitCode != 0 || rejected.Commit == "" {
		t.Fatalf("first execution failed: %+v", rejected)
	}
	job := first
	job.ExecutionID++
	job.Workspace = filepath.Join(w.workspaceRoot, fmt.Sprintf("execution-%d", job.ExecutionID))
	job.Rework = &store.Review{ExecutionID: first.ExecutionID, Commit: rejected.Commit, Decision: "reject", Note: "APPEND_EXAMPLE"}
	fixed := w.execute(ctx, job)
	if fixed.ExitCode != 0 || fixed.SessionID != "test-review-session" || fixed.BaseCommit != base ||
		fixed.Commit == "" || fixed.Commit == rejected.Commit || fixed.Validation == nil ||
		fixed.Validation.Commit != fixed.Commit || fixed.Validation.ExitCode != 0 {
		t.Fatalf("revision is missing linked execution evidence: %+v", fixed)
	}
	if git(t, job.Workspace, "show", "HEAD:README.md") != "An implemented change.\nExample: start here." ||
		git(t, job.Workspace, "rev-parse", "HEAD^") != base {
		t.Fatal("revision did not include both edits as one candidate on the frozen base")
	}
	if git(t, first.Workspace, "rev-parse", "HEAD") != rejected.Commit ||
		git(t, first.Workspace, "status", "--porcelain") != "" || git(t, first.RepoPath, "rev-parse", "HEAD") != base {
		t.Fatal("rework changed an existing checkout")
	}

	job.ExecutionID++
	job.Workspace = filepath.Join(w.workspaceRoot, fmt.Sprintf("execution-%d", job.ExecutionID))
	job.Rework.Note = "SIMULATED_NO_CHANGE"
	if result := w.execute(ctx, job); result.ExitCode == 0 || result.Commit != "" || result.Validation != nil {
		t.Fatalf("unchanged rejected tree reached acceptance: %+v", result)
	}
	job.ExecutionID++
	job.Workspace = filepath.Join(w.workspaceRoot, fmt.Sprintf("execution-%d", job.ExecutionID))
	job.Rework.Commit = base
	if result := w.execute(ctx, job); result.ExitCode == 0 || result.SessionID != "" {
		t.Fatalf("unrelated rework revision reached Codex: %+v", result)
	}
}

func TestRejectsExecutableCheckoutFilters(t *testing.T) {
	w, job, base := fixture(t)
	w.codex, _ = os.Executable()
	job.Kind = "codex_issue"
	job.Spec = &store.IssueSpec{BaseCommit: base, Validation: "docs"}
	git(t, job.RepoPath, "config", "filter.example.clean", "unused")
	res := w.execute(context.Background(), job)
	if res.ExitCode == 0 || !strings.Contains(res.Error, "Git filters") || res.SessionID != "" {
		t.Fatalf("unexpected filter rejection: %+v", res)
	}
}
