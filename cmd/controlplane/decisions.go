package main

import (
	"net/http"
	"strconv"

	"forgeos/internal/store"
)

func registerDecisions(mux *http.ServeMux, data *store.Store, owner string, workers workerAccess) {
	registerProjectRead(mux, "decisions", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		q := r.URL.Query()
		status := q.Get("status")
		switch status {
		case "":
			status = "CONFIRMED"
		case "ALL":
			status = ""
		case "PROPOSED", "CONFIRMED", "REJECTED", "SUPERSEDED", "EXPIRED":
		default:
			problem(w, 400, "invalid decision status")
			return
		}
		var before int64
		if q.Has("before") {
			var err error
			before, err = strconv.ParseInt(q.Get("before"), 10, 64)
			if err != nil || before < 1 {
				problem(w, 400, "invalid decision cursor")
				return
			}
		}
		out, err := data.Decisions(r.Context(), id, before, status)
		respond(w, 200, out, err)
	})
	mux.HandleFunc("POST /projects/{id}/decisions", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var input store.DecisionProposal
		if !decode(w, r, &input) {
			return
		}
		if !input.Valid() {
			problem(w, 400, "provide a request key, statement, source, validity and evidence references")
			return
		}
		out, err := data.ProposeDecision(r.Context(), id, input)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("POST /projects/{id}/decisions/{decision}/status", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		decision, err := strconv.ParseInt(r.PathValue("decision"), 10, 64)
		if err != nil || decision < 1 {
			problem(w, 400, "invalid decision id")
			return
		}
		var input struct {
			Status string `json:"status"`
		}
		if !decode(w, r, &input) {
			return
		}
		switch input.Status {
		case "CONFIRMED", "REJECTED", "EXPIRED":
		default:
			problem(w, 400, "choose CONFIRMED, REJECTED or EXPIRED")
			return
		}
		out, err := data.Decide(r.Context(), id, decision, input.Status)
		respond(w, 200, out, err)
	}))
}
