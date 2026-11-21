package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"forgeos/internal/localrepo"
	"forgeos/internal/store"
)

func observeDocuments(ctx context.Context, data *store.Store, root string, project int64) error {
	p, err := data.Project(ctx, project)
	if err != nil {
		return err
	}
	repo, err := localrepo.ResolveRepo(ctx, root, p.Path)
	if err != nil {
		return store.ErrConflict
	}
	return data.ObserveDocumentHead(ctx, project, repo)
}

func watchDocuments(ctx context.Context, data *store.Store, root string) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for ctx.Err() == nil {
		var before int64
		for ctx.Err() == nil {
			page, err := data.RegisteredProjects(ctx, before)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("list projects for document scans: %v", err)
				}
				break
			}
			for _, p := range page {
				run, stop := context.WithTimeout(ctx, 2*time.Minute)
				err := observeDocuments(run, data, root, p.ID)
				stop()
				if err != nil && ctx.Err() == nil {
					log.Printf("project %d document scan: %v", p.ID, err)
				}
			}
			if len(page) < 31 {
				break
			}
			before = page[len(page)-1].ID
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func registerDocumentChanges(mux *http.ServeMux, data *store.Store, root, owner string, workers *workerRegistry) {
	registerDocumentBrowser(mux, data, root, owner, workers)
	registerIndexedSearch(mux, "documents", owner, workers, func(w http.ResponseWriter, r *http.Request, project int64) {
		params := r.URL.Query()
		query, commit := strings.TrimSpace(params.Get("q")), params.Get("commit")
		if query == "" || len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) || (params.Has("commit") && !gitCommit.MatchString(commit)) {
			problem(w, 400, "provide a document query and, optionally, a full commit SHA")
			return
		}
		repo, ok := documentRepository(w, r, data, root, project)
		if !ok {
			return
		}
		result, err := data.SearchDocuments(r.Context(), project, repo, commit, query)
		if errors.Is(err, localrepo.ErrDocumentRevision) {
			problem(w, 422, "cannot search the requested repository revision")
			return
		}
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		writeJSON(w, 200, struct {
			Project int64  `json:"project_id"`
			Status  string `json:"status"`
			localrepo.DocumentSearch
		}{project, "committed", result})
	})
	const path = "/projects/{id}/document-changes"
	list := authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		var before int64
		if r.URL.Query().Has("before") {
			var err error
			before, err = strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
			if err != nil || before < 1 {
				problem(w, 400, "invalid document cursor")
				return
			}
		}
		if r.Method == http.MethodPost {
			if err := observeDocuments(r.Context(), data, root, project); err != nil {
				respond(w, 0, nil, err)
				return
			}
		}
		out, err := data.DocumentChanges(r.Context(), project, before)
		respond(w, 200, out, err)
	})
	mux.HandleFunc("GET "+path, list)
	mux.HandleFunc("POST "+path, list)
	mux.HandleFunc("GET "+path+"/{change}/curation", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		change, err := strconv.ParseInt(r.PathValue("change"), 10, 64)
		if err != nil || change < 1 {
			problem(w, 400, "invalid document change")
			return
		}
		var part int64
		if r.URL.Query().Has("part") {
			part, err = strconv.ParseInt(r.URL.Query().Get("part"), 10, 32)
			if err != nil || part < 1 {
				problem(w, 400, "invalid document part")
				return
			}
		}
		out, err := data.DocumentPartCuration(r.Context(), project, change, int(part))
		respond(w, 200, out, err)
	}))
}
