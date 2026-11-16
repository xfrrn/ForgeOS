package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

const ciStatusContext = "forgeos/local-ci"

func publishValidation(root string, task, execution int64, api deliveryAPI) store.StatusPublisher {
	return func(ctx context.Context, project string, issue store.Issue, v store.ValidationResult) (store.CommitStatus, error) {
		state := "success"
		if v.ExitCode != 0 {
			state = "failure"
			if v.Run != nil {
				switch v.Run.Failure {
				case "INFRA_FAILURE", "CONFIG_FAILURE", "TEST_ENV_FAILURE", "TIMEOUT":
					state = "error"
				}
			}
		}
		profile := v.Profile
		if profile == "auto" && v.Run != nil {
			profile = v.Run.Level
		}
		want := store.CommitStatus{Commit: v.Commit, Context: ciStatusContext, State: state, Description: fmt.Sprintf("Task %d / execution %d / %s", task, execution, profile)}
		if v.Run != nil {
			want.Description += fmt.Sprintf(" / %d checks", len(v.Run.Checks))
		}
		repo, err := localrepo.ResolveRepo(ctx, root, project)
		if err != nil {
			return want, errOrigin
		}
		origin, err := localrepo.Git(ctx, repo, "config", "--get", "remote.origin.url")
		if err != nil || !strings.EqualFold(repositoryName(strings.TrimSpace(string(origin))), issue.Repository) {
			return want, errIdentity
		}
		base := fmt.Sprintf("repositories/%d", issue.RepositoryID)
		// GitHub returns an empty combined status even for an unknown 40-character hash.
		var commit struct {
			SHA string `json:"sha"`
		}
		if err := api(ctx, "GET", base+"/git/commits/"+v.Commit, nil, &commit); err != nil {
			return want, err
		}
		if commit.SHA != v.Commit {
			return want, errIdentity
		}
		match := func(s store.CommitStatus) bool {
			return s.ID > 0 && s.Context == want.Context && s.State == want.State && s.Description == want.Description
		}
		for page := 1; ; page++ {
			var current struct {
				SHA        string               `json:"sha"`
				Repository githubProject        `json:"repository"`
				Statuses   []store.CommitStatus `json:"statuses"`
				Count      int                  `json:"total_count"`
			}
			err = api(ctx, "GET", fmt.Sprintf("%s/commits/%s/status?per_page=100&page=%d", base, v.Commit, page), nil, &current)
			if err != nil {
				return want, err
			}
			if current.SHA != v.Commit || current.Repository.ID != issue.RepositoryID || !strings.EqualFold(current.Repository.FullName, issue.Repository) {
				return want, errIdentity
			}
			found := false
			for _, s := range current.Statuses {
				if s.Context == want.Context {
					if match(s) {
						want.ID = s.ID
						return want, nil
					}
					found = true
					break
				}
			}
			if found || len(current.Statuses) < 100 || page*100 >= current.Count {
				break
			}
		}
		var posted store.CommitStatus
		err = api(ctx, "POST", base+"/statuses/"+v.Commit, map[string]string{"context": want.Context, "state": want.State, "description": want.Description}, &posted)
		if err != nil {
			return want, err
		}
		if !match(posted) {
			return want, errors.New("GitHub did not confirm the local CI status")
		}
		want.ID = posted.ID
		return want, nil
	}
}

func (p deliveryPolicy) validationStatus(data *store.Store, root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, ok := pathID(w, r)
		var input struct {
			Execution int64  `json:"execution_id"`
			Commit    string `json:"commit"`
		}
		if !ok || !decode(w, r, &input) {
			return
		}
		if p.projectID == 0 {
			problem(w, 403, "GitHub writes are not configured for a project")
			return
		}
		if input.Execution < 1 || !gitCommit.MatchString(input.Commit) {
			problem(w, 400, "provide the execution and full candidate SHA")
			return
		}
		receipt, err := data.PublishCommitStatus(r.Context(), p.projectID, task, input.Execution, input.Commit, publishValidation(root, task, input.Execution, deliveryRequest))
		switch {
		case errors.Is(err, errMissing):
			problem(w, 409, "candidate commit unavailable in the configured GitHub repository")
		case errors.Is(err, errIdentity), errors.Is(err, errOrigin):
			problem(w, 409, "repository identity does not match the candidate")
		case err != nil && !errors.Is(err, store.ErrConflict) && !errors.Is(err, store.ErrNotFound):
			problem(w, 502, "could not confirm publication; retry to reconcile GitHub and the stored receipt")
		default:
			respond(w, 200, receipt, err)
		}
	}
}
