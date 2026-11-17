package main

import (
	"net/http"
	"strconv"

	"forgeos/internal/store"
)

func registerGlobalRules(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	mux.HandleFunc("GET /global-rules", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var rev int64
		if r.URL.Query().Has("revision") {
			var err error
			rev, err = strconv.ParseInt(r.URL.Query().Get("revision"), 10, 32)
			if err != nil || rev < 1 {
				problem(w, 400, "invalid global rules revision")
				return
			}
		}
		out, err := data.GlobalRules(r.Context(), int(rev))
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("PUT /global-rules", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Previous *int    `json:"previous_revision"`
			Content  *string `json:"content"`
		}
		if !decodeLimited(w, r, &input, 128<<10) {
			return
		}
		if input.Previous == nil || input.Content == nil {
			problem(w, 400, "provide previous_revision and content")
			return
		}
		out, err := data.PublishGlobalRules(r.Context(), *input.Previous, *input.Content)
		respond(w, 200, out, err)
	}))
	registerProjectRead(mux, "global-rules", owner, workers, func(w http.ResponseWriter, r *http.Request, project int64) {
		if r.URL.Query().Has("revision") {
			problem(w, 400, "project reads return the current global rules")
			return
		}
		if _, err := data.Project(r.Context(), project); err != nil {
			respond(w, 0, nil, err)
			return
		}
		out, err := data.GlobalRules(r.Context(), 0)
		respond(w, 200, out, err)
	})
}
