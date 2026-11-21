package main

import (
	"forgeos/internal/store"
	"net/http"
)

func registerProjectPlanning(mux *http.ServeMux, c projectChat, owner string) {
	const path = "/projects/{id}/planning"
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		p, ok := pathID(w, r)
		if !ok {
			return
		}
		out, err := c.data.ProjectPlanning(r.Context(), p)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("GET "+path+"/tasks/{task}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		p, ok := pathID(w, r)
		if !ok {
			return
		}
		t, ok := experiencePathID(w, r, "task")
		if !ok {
			return
		}
		out, err := c.data.PlanningLinks(r.Context(), p, t)
		respond(w, 200, out, err)
	}))
	mux.HandleFunc("POST "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		p, ok := pathID(w, r)
		if !ok {
			return
		}
		var e store.PlanningEdit
		if !decodeLimited(w, r, &e, 1<<20) {
			return
		}
		if !e.Valid() {
			problem(w, 400, "请检查标题、正文和所选记录。")
			return
		}
		var repo githubProject
		if e.Action == "draft" {
			if !validChatIssue(e.Title, e.Body, "development") {
				problem(w, 400, "请检查 Issue 标题和正文长度。")
				return
			}
			var err error
			repo, _, err = c.issueRepository(r.Context(), p)
			if err != nil {
				problem(w, 502, "无法核对 GitHub 仓库，请检查网络和仓库凭据。")
				return
			}
		}
		if err := c.data.EditPlanning(r.Context(), p, e, repo.FullName, repo.ID); err != nil {
			respond(w, 0, nil, err)
			return
		}
		out, err := c.data.ProjectPlanning(r.Context(), p)
		respond(w, 200, out, err)
	}))
}
