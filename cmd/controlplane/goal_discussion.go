package main

import (
	"errors"
	"forgeos/internal/store"
	"net/http"
)

func registerGoalDiscussion(mux *http.ServeMux, data *store.Store, owner string) {
	const path = "/tasks/{id}/planner-discussion"
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		task, ok := pathID(w, r)
		if !ok {
			return
		}
		turns, err := data.DiscussionHistory(r.Context(), task)
		respond(w, 200, turns, err)
	}))
	mux.HandleFunc("POST "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		task, ok := pathID(w, r)
		if !ok {
			return
		}
		var in store.DiscussionInput
		if !decodeLimited(w, r, &in, 32<<10) {
			return
		}
		id, err := data.SendDiscussion(r.Context(), task, in)
		if errors.Is(err, store.ErrConflict) {
			problem(w, 409, "请核对最新讨论和计划修订后重试。")
			return
		}
		respond(w, 202, map[string]int64{"id": id}, err)
	}))
}
