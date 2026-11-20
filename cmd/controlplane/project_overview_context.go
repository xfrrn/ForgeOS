package main

import (
	"context"
	"net/http"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

type projectRepositoryStatus struct {
	projectCommit
	Dirty *bool `json:"dirty,omitempty"`
}

func registerProjectOverviewContext(mux *http.ServeMux, data *store.Store, root, owner string, pollers map[int64]*issuePoller) {
	mux.HandleFunc("GET /projects/{id}/overview-context", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		project, err := data.Project(r.Context(), id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		activity, activityErr := data.RecentProjectActivity(r.Context(), id)
		checks, checkErr := data.ProjectTestChecks(r.Context(), id, 0)
		var check *store.ProjectTestCheck
		if len(checks.Items) > 0 {
			check = &checks.Items[0]
		}
		github := "disabled"
		var poll *githubPollStatus
		if p := pollers[id]; p != nil {
			github = "pending"
			if poll = p.status.Load(); poll != nil {
				github = poll.State
			}
		}
		gitCtx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, struct {
			Repository     *projectRepositoryStatus `json:"repository,omitempty"`
			Check          *store.ProjectTestCheck  `json:"check,omitempty"`
			CheckFailed    bool                     `json:"check_failed,omitempty"`
			Activity       []store.ProjectActivity  `json:"activity"`
			ActivityFailed bool                     `json:"activity_failed,omitempty"`
			GitHub         string                   `json:"github"`
			Poll           *githubPollStatus        `json:"poll,omitempty"`
		}{readProjectRepositoryStatus(gitCtx, root, project.Path), check, checkErr != nil, activity, activityErr != nil, github, poll})
	}))
}

func readProjectRepositoryStatus(ctx context.Context, root, path string) *projectRepositoryStatus {
	repo, err := localrepo.ResolveRepo(ctx, root, path)
	if err != nil {
		return nil
	}
	head, err := readProjectCommit(ctx, repo)
	if err != nil {
		return nil
	}
	result := &projectRepositoryStatus{projectCommit: head}
	if out, _, err := localrepo.LimitedGit(ctx, repo, 1, "--no-optional-locks", "-c", "core.fsmonitor=false",
		"status", "--porcelain=v1", "--untracked-files=normal"); err == nil {
		dirty := len(out) > 0
		result.Dirty = &dirty
	}
	return result
}
