package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func TestDeliveryRequestErrorsReportOnlyStatus(t *testing.T) {
	for _, tc := range []struct{ stderr, want string }{
		{"gh: private remote response (HTTP 403)\r\n", "GitHub HTTP 403: check repository access, credentials and repository rules"},
		{"gh: private issue details (HTTP 422)\n", "GitHub HTTP 422: check remote state and repository rules"},
		{"gh: sensitive network error", "GitHub operation unconfirmed; will reconcile remote state on retry"},
	} {
		if got := deliveryRequestError(&exec.ExitError{Stderr: []byte(tc.stderr)}).Error(); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
	}
}

func deliveryGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := localrepo.Git(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

func deliveryFixture(t *testing.T) (string, string, store.Task, store.Delivery) {
	t.Helper()
	root, project, base := issueTestRepo(t)
	remote := filepath.Join(root, "upstream.git")
	deliveryGit(t, root, "init", "--bare", remote)
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("accepted content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, project, "add", "README.md")
	deliveryGit(t, project, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "candidate")
	sha := deliveryGit(t, project, "rev-parse", "HEAD")
	task := store.Task{ID: 3, Kind: "codex_issue", Spec: &store.IssueSpec{
		Issue:      store.Issue{ID: 77, Number: 7, RepositoryID: 42, Repository: "my-org/project", URL: "https://github.com/my-org/project/issues/7"},
		BaseCommit: base, Validation: "docs",
	}}
	d := store.Delivery{TaskID: 3, ExecutionID: 4, Branch: "forgeos/task-3-" + sha[:12], Commit: sha, State: "PENDING"}
	return project, remote, task, d
}

func TestDeliveryPushCreatesOnlyExpectedRef(t *testing.T) {
	repo, remote, task, d := deliveryFixture(t)
	ctx := context.Background()
	other := filepath.Join(t.TempDir(), "other.git")
	deliveryGit(t, filepath.Dir(other), "init", "--bare", other)
	deliveryGit(t, repo, "config", "url."+other+".insteadOf", remote)
	if err := pushFromIsolatedRepo(ctx, repo, remote, "file", d.Branch, d.Commit); err != nil {
		t.Fatal(err)
	}
	ref := "refs/heads/" + d.Branch
	if got := deliveryGit(t, remote, "rev-parse", ref); got != d.Commit {
		t.Fatalf("remote ref = %q", got)
	}
	if _, err := localrepo.Git(ctx, other, "rev-parse", "--verify", ref); err == nil {
		t.Fatal("push followed the project's URL rewrite")
	}
	if err := pushFromIsolatedRepo(ctx, repo, remote, "file", d.Branch, d.Commit); err != nil {
		t.Fatal("repeat:", err)
	}
	deliveryGit(t, remote, "update-ref", ref, task.Spec.BaseCommit, d.Commit)
	if err := pushFromIsolatedRepo(ctx, repo, remote, "file", d.Branch, d.Commit); err == nil {
		t.Fatal("accepted an existing branch that contains a different commit")
	}
	if got := deliveryGit(t, remote, "rev-parse", ref); got != task.Spec.BaseCommit {
		t.Fatal("existing remote ref was changed")
	}
}

func TestRemoteDeliveryReconcilesWritesAndGates(t *testing.T) {
	repo, remote, task, d := deliveryFixture(t)
	ctx := context.Background()
	i := task.Spec.Issue
	issuePath := "repositories/42/issues/7"
	issue := map[string]any{"id": i.ID, "number": i.Number, "state": "open", "html_url": i.URL,
		"repository_url": "https://api.github.com/repos/" + i.Repository}
	prPath := "repositories/42/pulls/9"
	var pull map[string]any
	var comments []map[string]string
	head := map[string]any{"ref": d.Branch, "sha": d.Commit, "repo": map[string]any{"id": i.RepositoryID}}
	mergedSHA := strings.Repeat("d", 40)
	var review any
	rollup, mergeState := "SUCCESS", "CLEAN"
	omitChecks := false
	targetSHA, mergeBase := task.Spec.BaseCommit, task.Spec.BaseCommit
	mergeParents := []map[string]string{{"sha": targetSHA}, {"sha": d.Commit}}
	var pushes, creates, merges, closes int
	lost := errors.New("response lost after side effect")
	api := func(_ context.Context, method, path string, body, dest any) error {
		var reply any
		switch {
		case method == "GET" && path == "repositories/42":
			reply = map[string]any{"id": i.RepositoryID, "full_name": i.Repository, "default_branch": "main"}
		case method == "GET" && path == issuePath:
			reply = issue
		case method == "GET" && path == "repositories/42/git/ref/heads/main":
			reply = map[string]any{"ref": "refs/heads/main", "object": map[string]any{"type": "commit", "sha": targetSHA}}
		case method == "GET" && path == "repositories/42/git/commits/"+mergedSHA:
			reply = map[string]any{"sha": mergedSHA, "parents": mergeParents}
		case method == "GET" && strings.Contains(path, "/compare/"):
			reply = map[string]any{"merge_base_commit": map[string]any{"sha": mergeBase}}
		case method == "GET" && strings.HasPrefix(path, "repositories/42/pulls?"):
			if pull == nil {
				reply = []any{}
			} else {
				reply = []any{pull}
			}
		case method == "POST" && path == "repositories/42/pulls":
			creates++
			args := body.(map[string]any)
			if args["head"] != d.Branch || args["base"] != "main" || !strings.Contains(args["body"].(string), d.Commit) {
				t.Fatal("PR does not identify the accepted candidate")
			}
			pull = map[string]any{"number": 9, "html_url": "https://github.com/my-org/project/pull/9", "head": head,
				"base": map[string]any{"ref": "main", "repo": map[string]any{"id": i.RepositoryID}}, "state": "open"}
			return lost
		case method == "GET" && path == prPath:
			reply = pull
		case method == "POST" && path == "graphql":
			commit := map[string]any{"oid": d.Commit, "statusCheckRollup": map[string]any{"state": rollup}}
			if omitChecks {
				delete(commit, "statusCheckRollup")
			}
			p := map[string]any{"headRefOid": d.Commit, "baseRefName": "main", "headRefName": d.Branch, "isDraft": false,
				"mergeable": "MERGEABLE", "mergeStateStatus": mergeState, "reviewDecision": review,
				"commits": map[string]any{"nodes": []any{map[string]any{"commit": commit}}}}
			reply = map[string]any{"data": map[string]any{"repository": map[string]any{"databaseId": 42, "pullRequest": p}}}
		case method == "PUT" && path == prPath+"/merge-async":
			args := body.(map[string]any)
			if args["sha"] != d.Commit || args["merge_method"] != "squash" || args["bypass_rules"] != false || args["merge_action"] != "direct_merge" {
				t.Fatal("missing squash/head/branch protection preconditions")
			}
			merges++
			pull["merged"], pull["state"], pull["merge_commit_sha"] = true, "closed", mergedSHA
			return lost
		case method == "GET" && strings.HasPrefix(path, issuePath+"/comments?"):
			reply = comments
		case method == "POST" && path == issuePath+"/comments":
			feedback := body.(map[string]any)["body"].(string)
			if !strings.Contains(feedback, "Merge commit: `"+mergedSHA+"`") || !strings.Contains(feedback, d.PRURL) {
				t.Fatal("feedback lacks PR or merge SHA")
			}
			comments = append(comments, map[string]string{"body": feedback})
			return lost
		case method == "PATCH" && path == issuePath:
			closes++
			if body.(map[string]any)["state"] != "closed" {
				t.Fatal("wrong Issue update")
			}
			issue["state"] = "closed"
			return lost
		default:
			return fmt.Errorf("unexpected %s %s", method, path)
		}
		raw, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, dest)
	}
	step := func() error {
		next, err := advanceRemote(ctx, repo, task, d, api, func(ctx context.Context, dir, slug, branch, sha string) error {
			pushes++
			if slug != i.Repository {
				t.Fatal("wrong push repository")
			}
			return pushFromIsolatedRepo(ctx, dir, remote, "file", branch, sha)
		})
		if err == nil {
			d = next
		}
		return err
	}
	if err := step(); err != nil || d.TargetBranch != "main" {
		t.Fatal(d, err)
	}
	targetSHA, mergeBase = strings.Repeat("a", 40), strings.Repeat("b", 40)
	if err := step(); err == nil || pushes != 0 {
		t.Fatal("unpublished base advanced")
	}
	mergeBase = task.Spec.BaseCommit
	if err := step(); !errors.Is(err, lost) {
		t.Fatal("create:", err)
	}
	if err := step(); err != nil || d.PRNumber != 9 || creates != 1 || pushes != 1 {
		t.Fatal("recover PR:", d, err)
	}
	head["sha"] = targetSHA
	if err := step(); err != nil || d.State != "WAITING" || merges != 0 {
		t.Fatal("head mismatch:", d, err)
	}
	head["sha"], review = d.Commit, "CHANGES_REQUESTED"
	if err := step(); err != nil || merges != 0 {
		t.Fatal("review gate:", err)
	}
	review, rollup, mergeState = "APPROVED", "FAILURE", "BLOCKED"
	if err := step(); err != nil || merges != 0 || d.Detail != "GitHub checks failed for the accepted head" {
		t.Fatal("CI gate:", err)
	}
	rollup, mergeState = "SUCCESS", "BLOCKED"
	if err := step(); err != nil || merges != 0 {
		t.Fatal("branch protection gate:", err)
	}
	mergeState, omitChecks = "CLEAN", true
	if err := step(); err != nil || merges != 0 {
		t.Fatal("incomplete gate response:", err)
	}
	review, omitChecks = nil, false
	if err := step(); !errors.Is(err, lost) || merges != 1 {
		t.Fatal("merge:", err)
	}
	if err := step(); err != nil || d.State != "WAITING" || d.MergeSHA != "" {
		t.Fatal("two-parent merge accepted:", d, err)
	}
	mergeParents = mergeParents[:1]
	if err := step(); err != nil || d.State != "MERGED" || d.MergeSHA != mergedSHA {
		t.Fatal("recover merge:", d, err)
	}
	if err := step(); !errors.Is(err, lost) || len(comments) != 1 || closes != 0 {
		t.Fatal("feedback:", err)
	}
	if err := step(); !errors.Is(err, lost) || len(comments) != 1 || closes != 1 {
		t.Fatal("close:", err)
	}
	if err := step(); err != nil || d.State != "DONE" || len(comments) != 1 || closes != 1 || merges != 1 {
		t.Fatal("final:", d, err)
	}
}
