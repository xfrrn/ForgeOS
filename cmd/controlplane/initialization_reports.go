package main

import (
	"net/http"
	"strconv"

	"forgeos/internal/store"
)

func registerInitializationReports(mux *http.ServeMux, data *store.Store, owner string) {
	const reports = "/tasks/{id}/initialization-reports"
	mux.HandleFunc("POST "+reports, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		doc, err := data.CaptureInitializationReport(r.Context(), id)
		respond(w, 200, doc, err)
	}))
	mux.HandleFunc("GET "+reports, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		rows, err := data.InitializationReports(r.Context(), id)
		respond(w, 200, rows, err)
	}))
	mux.HandleFunc("GET "+reports+"/{revision}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		revision, err := strconv.ParseInt(r.PathValue("revision"), 10, 32)
		if err != nil || revision < 1 {
			problem(w, 400, "invalid report revision")
			return
		}
		doc, err := data.InitializationReport(r.Context(), id, int(revision))
		respond(w, 200, doc, err)
	}))
}
