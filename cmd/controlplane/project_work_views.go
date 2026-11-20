package main

import (
	"net/http"
	"strconv"

	"forgeos/internal/store"
)

func registerProjectWorkViews(mux *http.ServeMux, data *store.Store, owner string) {
	mux.HandleFunc("GET /projects/{id}/execution-queue", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		page, err := data.ProjectExecutionQueue(r.Context(), id)
		respond(w, http.StatusOK, page, err)
	}))
	mux.HandleFunc("GET /projects/{id}/test-checks", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var before int64
		if values, found := r.URL.Query()["before"]; found {
			var err error
			before, err = strconv.ParseInt(values[0], 10, 64)
			if len(values) != 1 || err != nil || before < 1 {
				problem(w, http.StatusBadRequest, "before must be a positive execution ID")
				return
			}
		}
		page, err := data.ProjectTestChecks(r.Context(), id, before)
		respond(w, http.StatusOK, page, err)
	}))
}
