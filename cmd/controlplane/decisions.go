package main

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"forgeos/internal/store"
)

func decisionStatus(w http.ResponseWriter, r *http.Request) (string, bool) {
	switch status := r.URL.Query().Get("status"); status {
	case "":
		return "CONFIRMED", true
	case "ALL":
		return "", true
	case "PROPOSED", "CONFIRMED", "REJECTED", "SUPERSEDED", "EXPIRED":
		return status, true
	default:
		problem(w, 400, "invalid decision status")
		return "", false
	}
}

func registerDecisions(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	registerProjectRead(mux, "decisions/search", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		status, ok := decisionStatus(w, r)
		if !ok {
			return
		}
		q := r.URL.Query()
		query := strings.TrimSpace(q.Get("q"))
		if query == "" || q.Has("before") || len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) {
			problem(w, 400, "invalid decision search query or cursor")
			return
		}
		out, err := data.SearchDecisions(r.Context(), id, status, query)
		respond(w, 200, out, err)
	})
	registerProjectRead(mux, "decisions/{decision}", owner, workers, func(w http.ResponseWriter, r *http.Request, project int64) {
		id, err := strconv.ParseInt(r.PathValue("decision"), 10, 64)
		if err != nil || id < 1 {
			problem(w, 400, "invalid decision id")
			return
		}
		if r.URL.Query().Get("versions") == "1" {
			out, err := data.DecisionWithVersions(r.Context(), project, id)
			respond(w, 200, out, err)
			return
		}
		out, err := data.Decision(r.Context(), project, id)
		respond(w, 200, out, err)
	})
	registerProjectRead(mux, "decisions", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		q := r.URL.Query()
		status, ok := decisionStatus(w, r)
		if !ok {
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
