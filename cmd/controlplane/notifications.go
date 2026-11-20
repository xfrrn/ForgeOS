package main

import (
	"net/http"
	"strconv"

	"forgeos/internal/store"
)

func registerNotifications(mux *http.ServeMux, data *store.Store, owner string) {
	mux.HandleFunc("GET /tasks/notifications", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		q, project, ok := ownerProjectQuery(w, r)
		if !ok {
			return
		}
		var before int64
		if q.Has("before") {
			var err error
			before, err = strconv.ParseInt(q.Get("before"), 10, 64)
			if err != nil || before < 1 {
				problem(w, 400, "invalid notification cursor")
				return
			}
		}
		scope := q.Get("scope")
		if scope != "" && scope != "pending" && scope != "history" {
			problem(w, 400, "invalid notification scope")
			return
		}
		page, err := data.Notifications(r.Context(), project, before, scope == "history")
		respond(w, 200, page, err)
	}))
	mux.HandleFunc("GET /tasks/notifications/unread", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		if _, project, ok := ownerProjectQuery(w, r); ok {
			n, err := data.UnreadNotifications(r.Context(), project)
			respond(w, 200, map[string]int64{"unread": n}, err)
		}
	}))
	mux.HandleFunc("POST /tasks/notifications/{id}/read", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		respond(w, 204, nil, data.ReadNotification(r.Context(), id))
	}))
}
