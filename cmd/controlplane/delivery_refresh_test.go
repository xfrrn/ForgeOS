package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
	"forgeos/internal/testdb"
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

func TestRefreshUsesCurrentTargetInsteadOfStalePullBase(t *testing.T) {
	ctx := context.Background()
	repo, _, fixture, candidate := deliveryFixture(t)
	base := fixture.Spec.BaseCommit
	deliveryGit(t, repo, "checkout", "--detach", base)
	if err := os.WriteFile(filepath.Join(repo, "other.md"), []byte("target change\n"), 0600); err != nil {
		t.Fatal(err)
	}
	deliveryGit(t, repo, "add", "other.md")
	deliveryGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "-qm", "advance")
	first := deliveryGit(t, repo, "rev-parse", "HEAD")
	deliveryGit(t, repo, "-c", "user.name=Test", "-c", "user.email=test@localhost", "commit", "--allow-empty", "-qm", "advance again")
	second := deliveryGit(t, repo, "rev-parse", "HEAD")
	data := store.New(testdb.Open(t))
	if err := data.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := data.CreateProject(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	task, err := data.ImportIssue(ctx, p.ID, *fixture.Spec)
	if err != nil {
		t.Fatal(err)
	}
	job, err := data.Claim(ctx, "writer", t.TempDir(), true)
	if err != nil || job == nil {
		t.Fatal(job, err)
	}
	if err = data.Complete(ctx, job.ExecutionID, job.LeaseToken, store.Result{Commit: candidate.Commit, BaseCommit: base, SessionID: "implementation", Validation: &store.ValidationResult{Commit: candidate.Commit, Profile: "docs"}}); err != nil {
		t.Fatal(err)
	}
	const branch = "release/v1"
	if _, err = data.Review(ctx, task.ID, job.ExecutionID, candidate.Commit, "approve", "checked", true, branch); err != nil {
		t.Fatal(err)
	}
	d, err := data.ClaimDelivery(ctx, p.ID)
	if err != nil || d == nil {
		t.Fatal(d, err)
	}
	d.State, d.PRNumber, d.PRURL = "WAITING", 9, "https://github.com/my-org/project/pull/9"
	if err = data.SaveDelivery(ctx, *d); err != nil {
		t.Fatal(err)
	}
	i := fixture.Spec.Issue
	pull := remotePull{Number: 9, HTMLURL: d.PRURL, State: "open"}
	pull.Head.Ref, pull.Head.SHA, pull.Head.Repo.ID = d.Branch, d.Commit, i.RepositoryID
	pull.Base.Ref, pull.Base.SHA, pull.Base.Repo.ID = branch, base, i.RepositoryID
	tip, ref, moveOnClose, closes, calls := base, "refs/heads/"+branch, false, 0, 0
	api := func(_ context.Context, method, path string, body, dest any) error {
		calls++
		var reply any
		switch {
		case method == "GET" && path == "repositories/42/issues/7":
			reply = map[string]any{"id": i.ID, "number": i.Number, "repository_url": "https://api.github.com/repos/" + i.Repository, "html_url": i.URL, "state": "open", "title": "Update guide", "updated_at": "2026-11-18T00:00:00Z"}
		case method == "GET" && path == "repositories/42/git/ref/heads/release%2Fv1":
			reply = map[string]any{"ref": ref, "object": map[string]string{"type": "commit", "sha": tip}}
		case path == "repositories/42/pulls/9":
			if method == "PATCH" {
				if body.(map[string]any)["state"] != "closed" {
					t.Fatal(body)
				}
				closes++
				pull.State = "closed"
				if moveOnClose {
					tip = second
				}
			} else if method != "GET" {
				t.Fatal(method)
			}
			reply = pull
		default:
			t.Fatalf("unexpected GitHub call %s %s", method, path)
		}
		raw, err := json.Marshal(reply)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, dest)
	}
	handler := (deliveryTargets{p.ID: branch}).refresh(data, filepath.Dir(repo), api)
	request := func(want int) store.Task {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"execution_id": job.ExecutionID, "commit": candidate.Commit})
		r := httptest.NewRequest("POST", "/", bytes.NewReader(body))
		r.SetPathValue("id", strconv.FormatInt(task.ID, 10))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != want {
			t.Fatalf("HTTP %d after %d remote calls: %s", w.Code, calls, w.Body.String())
		}
		got, err := data.Task(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	request(409)
	ref = "refs/heads/other"
	request(502)
	if closes != 0 {
		t.Fatal("closed PR before confirming the target")
	}
	ref, tip, moveOnClose = "refs/heads/"+branch, first, true
	got := request(409)
	if got.Status != "ACCEPTED" || len(got.Refreshes) != 0 || closes != 1 {
		t.Fatal("queued a candidate after target changed during closure")
	}
	got = request(200)
	if got.Status != "READY" || got.Spec.BaseCommit != second || len(got.Refreshes) != 1 || closes != 1 || got.Delivery != nil {
		t.Fatal(got)
	}
	sha := got.Spec.VerifyCommit
	if deliveryGit(t, repo, "rev-parse", sha+"^") != second || deliveryGit(t, repo, "diff", "--name-only", second, sha) != "README.md" || deliveryGit(t, repo, "show", sha+":README.md") != "accepted content" {
		t.Fatal("refreshed wrong content or base")
	}
	before := calls
	request(200)
	if calls != before {
		t.Fatal("replay repeated remote effects")
	}
}
