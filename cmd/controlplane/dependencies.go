package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func (p deliveryPolicy) releaseDependent(ctx context.Context, data *store.Store, root string, after *int64) (bool, error) {
	job, err := data.NextDependent(ctx, p.projectID, *after)
	if err != nil || job == nil {
		return false, err
	}
	// A diverged branch in one goal must not starve other eligible work.
	*after = job.TaskID
	project, err := data.Project(ctx, p.projectID)
	if err != nil {
		return false, err
	}
	repo, err := localrepo.ResolveRepo(ctx, root, project.Path)
	if err != nil {
		return false, err
	}
	if job.Issue.RepositoryID != project.RepositoryID || (p.target != "" && job.Target != p.target) {
		return false, errors.New("dependent item requires the original project and target branch")
	}
	origin, err := localrepo.Git(ctx, repo, "config", "--get", "remote.origin.url")
	slug := repositoryName(strings.TrimSpace(string(origin)))
	if err != nil || !strings.EqualFold(slug, job.Issue.Repository) {
		return false, errIdentity
	}
	var tip struct {
		Ref    string
		Object struct{ Type, SHA string }
	}
	if err := ghRead(ctx, fmt.Sprintf("repositories/%d/git/ref/heads/%s", project.RepositoryID, url.PathEscape(job.Target)), &tip); err != nil {
		return false, err
	}
	if tip.Ref != "refs/heads/"+job.Target || tip.Object.Type != "commit" || !gitCommit.MatchString(tip.Object.SHA) {
		return false, errIdentity
	}
	sha := tip.Object.SHA
	if err := fetchBase(ctx, repo, root, slug, sha); err != nil {
		return false, err
	}
	for _, ancestor := range append(job.Merges, job.Base) {
		if !gitCommit.MatchString(ancestor) {
			return false, errIdentity
		}
		if _, err := localrepo.Git(ctx, repo, "merge-base", "--is-ancestor", ancestor, sha); err != nil {
			return false, errors.New("dependency merge is absent from the target branch history")
		}
	}
	// Pin the verified object before publishing the job, including across startup or repository GC.
	ref := fmt.Sprintf("refs/forgeos/bases/%d/%s", job.TaskID, sha)
	if _, err := localrepo.Git(ctx, repo, "update-ref", "--no-deref", ref, sha); err != nil {
		return false, err
	}
	return true, data.ReleaseDependent(ctx, job.TaskID, job.Revision, sha)
}

// Fetch into the authorized object store with an isolated Git config and a fixed GitHub transport.
func fetchBase(ctx context.Context, repo, root, slug, sha string) error {
	if _, err := localrepo.Git(ctx, repo, "cat-file", "-e", sha+"^{commit}"); err == nil {
		return nil
	}
	raw, err := localrepo.Git(ctx, repo, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return err
	}
	objects, err := filepath.EvalSymlinks(strings.TrimSpace(string(raw)))
	if err != nil {
		return err
	}
	allowed := localrepo.ContainingRoot(root, repo)
	rel, err := filepath.Rel(allowed, objects)
	if allowed == "" || err != nil || !filepath.IsLocal(rel) {
		return errors.New("project object store is outside the authorized root")
	}
	tmp, err := os.MkdirTemp("", "forgeos-fetch-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	format := "sha1"
	if len(sha) == 64 {
		format = "sha256"
	}
	if _, err = localrepo.Git(ctx, tmp, "init", "--bare", "--template=", "--object-format="+format, "."); err != nil {
		return err
	}
	cmd := localrepo.GitCommand(ctx, tmp, "-c", "protocol.https.allow=always", "-c", "http.followRedirects=false",
		"-c", "credential.helper=", "-c", "credential.https://github.com.helper=!gh auth git-credential",
		"fetch", "--no-tags", "--no-recurse-submodules", "--no-write-fetch-head", "--no-auto-maintenance", "https://github.com/"+slug+".git", sha)
	cmd.Env = append(cmd.Env, "GIT_OBJECT_DIRECTORY="+objects)
	if err = cmd.Run(); err != nil {
		return errors.New("could not retrieve the verified dependency base")
	}
	return nil
}
