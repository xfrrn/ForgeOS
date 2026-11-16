package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

// Apply the accepted patch to a private index. An unapplicable patch leaves the checkout and remote PR intact.
func refreshCommit(ctx context.Context, repo, oldBase, commit, newBase string) (string, error) {
	for _, sha := range []string{oldBase, commit, newBase} {
		if !gitCommit.MatchString(sha) {
			return "", errIdentity
		}
	}
	if oldBase == newBase {
		return "", store.ErrConflict
	}
	if _, err := localrepo.Git(ctx, repo, "merge-base", "--is-ancestor", oldBase, newBase); err != nil {
		return "", store.ErrConflict
	}
	parent, err := localrepo.Git(ctx, repo, "show", "-s", "--format=%P", commit)
	if err != nil || strings.TrimSpace(string(parent)) != oldBase {
		return "", store.ErrConflict
	}
	dir, err := os.MkdirTemp("", "forgeos-refresh-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	patch, err := os.Create(filepath.Join(dir, "accepted.patch"))
	if err != nil {
		return "", err
	}
	defer patch.Close()
	diff := localrepo.GitCommand(ctx, repo, "diff", "--no-ext-diff", "--no-textconv", "--binary", "--full-index", "--no-renames", oldBase, commit, "--")
	diff.Stdout = patch
	if err = diff.Run(); err != nil {
		return "", err
	}
	if _, err = patch.Seek(0, 0); err != nil {
		return "", err
	}
	run := func(args ...string) (string, error) {
		cmd := localrepo.GitCommand(ctx, repo, args...)
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+filepath.Join(dir, "index"))
		if args[0] == "apply" {
			cmd.Stdin = patch
		}
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	if _, err = run("read-tree", newBase); err != nil {
		return "", err
	}
	if _, err = run("apply", "--cached", "--binary", "--whitespace=nowarn", "-"); err != nil {
		return "", store.ErrConflict
	}
	tree, err := run("write-tree")
	if err != nil {
		return "", err
	}
	// A stable date makes retries after an interrupted HTTP response produce the same SHA.
	date, err := localrepo.Git(ctx, repo, "show", "-s", "--format=%cI", commit)
	if err != nil {
		return "", err
	}
	cmd := localrepo.GitCommand(ctx, repo, "-c", "user.name=ForgeOS", "-c", "user.email=delivery@forgeos.local", "commit-tree", "--no-gpg-sign", tree, "-p", newBase, "-m", "Refresh accepted candidate "+commit)
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+strings.TrimSpace(string(date)), "GIT_COMMITTER_DATE="+strings.TrimSpace(string(date)))
	raw, err := cmd.Output()
	if err != nil {
		return "", err
	}
	sha := strings.TrimSpace(string(raw))
	if !gitCommit.MatchString(sha) {
		return "", errIdentity
	}
	_, err = localrepo.Git(ctx, repo, "update-ref", "--no-deref", "refs/forgeos/refresh/"+sha, sha)
	return sha, err
}

func (p deliveryPolicy) refresh(data *store.Store, root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			ExecutionID int64  `json:"execution_id"`
			Commit      string `json:"commit"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.ExecutionID <= 0 || !gitCommit.MatchString(input.Commit) {
			problem(w, http.StatusBadRequest, "provide the accepted execution and commit")
			return
		}
		ctx := r.Context()
		task, err := data.Task(ctx, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if p.projectID <= 0 || task.ProjectID != p.projectID {
			problem(w, http.StatusForbidden, "project is not enabled for delivery")
			return
		}
		for _, prior := range task.Refreshes {
			if prior.ExecutionID == input.ExecutionID && prior.OldDelivery.Commit == input.Commit {
				respond(w, http.StatusOK, task, nil)
				return
			}
		}
		d := task.Delivery
		if d == nil || task.Spec == nil || task.Status != "ACCEPTED" || d.State != "WAITING" || d.PRNumber <= 0 || d.Commit != input.Commit || d.ExecutionID != input.ExecutionID || (p.target != "" && p.target != d.TargetBranch) {
			respond(w, 0, nil, store.ErrConflict)
			return
		}
		project, err := data.Project(ctx, task.ProjectID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(ctx, root, project.Path)
		i := task.Spec.Issue
		if err != nil || project.RepositoryID != i.RepositoryID {
			respond(w, 0, nil, store.ErrConflict)
			return
		}
		origin, err := localrepo.Git(ctx, repo, "remote", "get-url", "origin")
		if err != nil || !strings.EqualFold(repositoryName(strings.TrimSpace(string(origin))), i.Repository) {
			respond(w, 0, nil, store.ErrConflict)
			return
		}
		var source githubIssue
		if err = ghRead(ctx, fmt.Sprintf("repositories/%d/issues/%d", i.RepositoryID, i.Number), &source); err != nil {
			problem(w, http.StatusBadGateway, "could not read delivery issue")
			return
		}
		identity := githubProject{ID: i.RepositoryID, FullName: i.Repository}
		if current, err := identity.issue(source, i.Number); err != nil || current.ID != i.ID {
			respond(w, 0, nil, store.ErrConflict)
			return
		}
		path := fmt.Sprintf("repositories/%d/pulls/%d", i.RepositoryID, d.PRNumber)
		var pr remotePull
		if err = deliveryRequest(ctx, "GET", path, nil, &pr); err != nil {
			problem(w, http.StatusBadGateway, "could not read the original PR")
			return
		}
		if !pr.matches(i, *d) || pr.Merged || (pr.State != "open" && pr.State != "closed") || !gitCommit.MatchString(pr.Base.SHA) || pr.Base.SHA == task.Spec.BaseCommit {
			respond(w, 0, nil, store.ErrConflict)
			return
		}
		base := pr.Base.SHA
		if err = fetchBase(ctx, repo, root, i.Repository, base); err != nil {
			problem(w, http.StatusBadGateway, "could not fetch the new target commit")
			return
		}
		candidate, err := refreshCommit(ctx, repo, task.Spec.BaseCommit, d.Commit, base)
		if err != nil {
			if errors.Is(err, store.ErrConflict) {
				problem(w, http.StatusConflict, "the accepted patch needs manual reconciliation with the target")
			} else {
				problem(w, http.StatusConflict, "could not prepare the refreshed candidate")
			}
			return
		}
		if pr.State == "open" {
			if err = deliveryRequest(ctx, "PATCH", path, map[string]any{"state": "closed"}, &pr); err != nil {
				problem(w, http.StatusBadGateway, "PR closure not confirmed; retry the same refresh")
				return
			}
		}
		if !pr.matches(i, *d) || pr.Merged || pr.State != "closed" || pr.Base.SHA != base {
			respond(w, 0, nil, store.ErrConflict)
			return
		}
		updated, err := data.QueueRefreshedCandidate(ctx, id, input.ExecutionID, d.Commit, base, candidate)
		respond(w, http.StatusOK, updated, err)
	}
}
