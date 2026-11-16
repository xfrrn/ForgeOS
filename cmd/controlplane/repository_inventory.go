package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func registerRepositoryInventory(mux *http.ServeMux, data *store.Store, root, owner string) {
	const checks = "/tasks/{id}/startup-checks"
	mux.HandleFunc("GET "+checks, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		history, err := data.StartupChecks(r.Context(), id)
		respond(w, 200, history, err)
	}))
	mux.HandleFunc("POST "+checks, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		var req struct {
			Environment string `json:"environment"`
			Previous    int64  `json:"previous_task_id"`
		}
		if !ok || !decode(w, r, &req) {
			return
		}
		if (req.Environment != "local-mock" && req.Environment != "local-integrated") || req.Previous < 0 {
			problem(w, 400, "provide a declared local environment and a valid previous task")
			return
		}
		task, err := data.QueueStartup(r.Context(), id, req.Environment, req.Previous)
		respond(w, 200, map[string]int64{"id": task}, err)
	}))
	const path = "/tasks/{id}/repository-inventory"
	serve := authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		doc, err := data.RepositoryInventory(ctx, id)
		if r.Method == http.MethodGet || !errors.Is(err, store.ErrNotFound) {
			respond(w, 200, doc, err)
			return
		}
		t, err := data.Task(ctx, id)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if t.Kind != "git_head" || t.Status != "DONE" || t.Execution == nil || t.Execution.Result == nil || t.Execution.Result.ExitCode != 0 {
			problem(w, 409, "repository inventory requires a successful HEAD check")
			return
		}
		p, err := data.Project(ctx, t.ProjectID)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		repo, err := localrepo.ResolveRepo(ctx, root, p.Path)
		if err != nil {
			problem(w, 409, "registered repository unavailable")
			return
		}
		commit := t.Execution.Result.Commit
		files, err := localrepo.Inventory(ctx, repo, commit)
		if err != nil {
			problem(w, 422, err.Error())
			return
		}
		var slug string
		if raw, err := localrepo.Git(ctx, repo, "remote", "get-url", "origin"); err == nil {
			slug = repositoryName(strings.TrimSpace(string(raw)))
		}
		raw, err := json.Marshal(files)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		doc, err = data.SaveRepositoryInventory(ctx, id, t.Execution.ID, commit, slug, raw)
		respond(w, 200, doc, err)
	})
	mux.HandleFunc("GET "+path, serve)
	mux.HandleFunc("POST "+path, serve)
}
