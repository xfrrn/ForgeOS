package main

import (
	"net/http"
	"strconv"
	"strings"

	"forgeos/internal/store"
)

func approvalKind(kind string) bool {
	return kind == "plan" || kind == "decision" || kind == "share" || kind == "question" || kind == "command" || kind == "patch"
}

func registerApprovals(mux *http.ServeMux, data *store.Store, owner string) {
	mux.HandleFunc("GET /tasks/approvals", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		kind := q.Get("kind")
		if kind != "" && !approvalKind(kind) {
			problem(w, 400, "unknown approval operation")
			return
		}
		var beforeKind string
		var beforeID int64
		if q.Has("before") {
			k, raw, ok := strings.Cut(q.Get("before"), ".")
			id, err := strconv.ParseInt(raw, 10, 64)
			if !ok || !approvalKind(k) || (kind != "" && k != kind) || err != nil || id < 1 {
				problem(w, 400, "invalid approval cursor")
				return
			}
			beforeKind, beforeID = k, id
		}
		questions := q.Get("include_questions") == "1" || kind == "question" || beforeKind == "question"
		page, err := data.PendingApprovals(r.Context(), kind, beforeKind, beforeID, questions, q.Get("include_commands") == "1" || kind == "command" || beforeKind == "command", q.Get("include_patches") == "1" || kind == "patch" || beforeKind == "patch")
		respond(w, 200, page, err)
	}))
}
