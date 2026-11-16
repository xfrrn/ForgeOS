package main

import (
	"net/http"
	"strconv"
	"strings"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func registerPlanRoutes(mux *http.ServeMux, data *store.Store, root, owner string) {
	mux.HandleFunc("POST /tasks/{id}/plans/{revision}/activate", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		revision, err := strconv.ParseInt(r.PathValue("revision"), 10, 32)
		if err != nil || revision < 1 {
			problem(w, http.StatusBadRequest, "provide a positive plan revision")
			return
		}
		task, err := data.ActivatePlan(r.Context(), id, int(revision))
		respond(w, http.StatusOK, task, err)
	}))
	mux.HandleFunc("POST /tasks/{id}/plans", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			Previous *int       `json:"previous_revision"`
			Plan     store.Plan `json:"plan"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Previous == nil || *input.Previous < 0 || *input.Previous >= 2147483647 {
			problem(w, http.StatusBadRequest, "provide a non-negative previous revision")
			return
		}
		if err := input.Plan.Validate(); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		rev, err := data.SavePlan(r.Context(), id, *input.Previous, input.Plan)
		respond(w, http.StatusOK, rev, err)
	}))
	mux.HandleFunc("GET /tasks/{id}/plans/{revision}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		n, err := strconv.ParseInt(r.PathValue("revision"), 10, 32)
		if err != nil || n < 1 {
			problem(w, http.StatusBadRequest, "provide a positive plan revision")
			return
		}
		rev, err := data.PlanRevision(r.Context(), id, n)
		respond(w, http.StatusOK, rev, err)
	}))
	mux.HandleFunc("POST /tasks/{id}/plans/{revision}/archive", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		n, err := strconv.ParseInt(r.PathValue("revision"), 10, 32)
		if err != nil || n < 1 {
			problem(w, http.StatusBadRequest, "provide a positive plan revision")
			return
		}
		ctx := r.Context()
		rev, err := data.PlanRevision(ctx, id, n)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		task, err := data.Task(ctx, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		project, err := data.Project(ctx, task.ProjectID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(ctx, root, project.Path)
		if err != nil || task.Spec == nil || task.Spec.Issue.RepositoryID != project.RepositoryID {
			problem(w, http.StatusConflict, "plan repository is unavailable or has changed")
			return
		}
		origin, err := localrepo.Git(ctx, repo, "config", "--get", "remote.origin.url")
		if err != nil || !strings.EqualFold(repositoryName(strings.TrimSpace(string(origin))), task.Spec.Issue.Repository) {
			problem(w, http.StatusConflict, "plan repository origin has changed")
			return
		}
		sha, err := archivePlan(ctx, repo, task.Spec.BaseCommit, rev)
		if err != nil {
			problem(w, http.StatusConflict, "could not archive the plan against its frozen base and revision")
			return
		}
		err = data.RecordPlanArchive(ctx, id, n, sha)
		rev.GitCommit = sha
		respond(w, http.StatusOK, rev, err)
	}))
}
