package main

import (
	"net/http"
	"strconv"

	"forgeos/internal/store"
)

func registerPlanRoutes(mux *http.ServeMux, data *store.Store, owner string) {
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
}
