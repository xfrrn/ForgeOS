package main

import (
	"net/http"

	"forgeos/internal/store"
)

func registerPreviewFeedback(mux *http.ServeMux, data *store.Store, owner string) {
	const path = "/tasks/{id}/previews/{preview}/feedback"
	registerFeedbackArtifacts(mux, data, owner, path)
	mux.HandleFunc("POST "+path+"/requirements", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var f store.FeedbackRequirement
		if !decodeLimited(w, r, &f, 2<<20) {
			return
		}
		if !f.Valid() {
			problem(w, 400, "provide the classified feedback, execution, candidate, previous revision and revised requirement body")
			return
		}
		rev, err := data.ReviseFromFeedback(r.Context(), id, r.PathValue("preview"), f)
		respond(w, http.StatusOK, rev, err)
	}))
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		page, err := data.PreviewFeedbackPage(r.Context(), id, r.PathValue("preview"))
		respond(w, 200, page, err)
	}))
	mux.HandleFunc("POST "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var f store.PreviewFeedback
		if !decodeLimited(w, r, &f, 32<<10) {
			return
		}
		if !f.Valid() {
			problem(w, 400, "provide a plain page path, supported browser metadata, a request ID, and a nonempty note")
			return
		}
		n, err := data.SubmitPreviewFeedback(r.Context(), id, r.PathValue("preview"), f)
		respond(w, 200, map[string]int64{"id": n}, err)
	}))
}
