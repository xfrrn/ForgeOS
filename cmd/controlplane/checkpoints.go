package main

import (
	"forgeos/internal/store"
	"net/http"
	"strconv"
)

func registerCheckpoints(mux *http.ServeMux, data *store.Store, owner, worker string) {
	mux.HandleFunc("GET /tasks/{id}/checkpoints", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		execution, err := strconv.ParseInt(r.URL.Query().Get("execution_id"), 10, 64)
		if err != nil || execution < 1 {
			problem(w, http.StatusBadRequest, "provide a positive execution ID")
			return
		}
		rows, err := data.Checkpoints(r.Context(), id, execution)
		respond(w, http.StatusOK, rows, err)
	}))
	mux.HandleFunc("POST /internal/executions/{id}/checkpoints", authorized(worker, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input struct {
			Lease    string `json:"lease_token"`
			Sequence *int   `json:"sequence"`
			After    int64  `json:"after"`
			Session  string `json:"session_id"`
		}
		if !decode(w, r, &input) {
			return
		}
		if input.Sequence == nil || *input.Sequence < 0 || *input.Sequence > 2147483647 || input.After < 0 || input.Lease == "" {
			problem(w, http.StatusBadRequest, "provide an execution lease and checkpoint sequence")
			return
		}
		page, err := data.ReadCheckpoint(r.Context(), id, input.Lease, *input.Sequence, input.After, input.Session)
		if input.Session != "" {
			respond(w, http.StatusNoContent, nil, err)
			return
		}
		respond(w, http.StatusOK, page, err)
	}))
}
