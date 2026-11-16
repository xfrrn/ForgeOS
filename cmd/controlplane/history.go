package main

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"forgeos/internal/store"
)

func registerHistory(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	search := func(w http.ResponseWriter, r *http.Request, project int64) {
		q := r.URL.Query()
		source, text := q.Get("source"), strings.TrimSpace(q.Get("q"))
		if (source != "" && source != "requirement" && source != "comment") || !utf8.ValidString(text) || strings.IndexByte(text, 0) >= 0 || len(text) > 512 {
			problem(w, http.StatusBadRequest, "invalid history source or query")
			return
		}
		var task int64
		if q.Has("task_id") {
			var err error
			task, err = strconv.ParseInt(q.Get("task_id"), 10, 64)
			if err != nil || task < 1 {
				problem(w, http.StatusBadRequest, "invalid history task ID")
				return
			}
		}
		rows, err := data.SearchHistory(r.Context(), project, task, source, text)
		respond(w, http.StatusOK, rows, err)
	}
	registerProjectRead(mux, "history", owner, workers, search)
}

func registerProjectRead(mux *http.ServeMux, resource, owner string, workers *workerRegistry, read func(http.ResponseWriter, *http.Request, int64)) {
	mux.HandleFunc("GET /projects/{id}/"+resource, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			read(w, r, id)
		}
	}))
	mux.HandleFunc("GET /internal/projects/{id}/"+resource, workers.authorize(func(w http.ResponseWriter, r *http.Request, grant *workerGrant) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if !slices.Contains(grant.projects, id) {
			problem(w, http.StatusForbidden, "project is outside this Worker's grant")
			return
		}
		read(w, r, id)
	}))
}
