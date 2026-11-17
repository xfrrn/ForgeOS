package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

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

func registerDocumentChanges(mux *http.ServeMux, data *store.Store, root, owner string) {
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
		out, err := data.DocumentCuration(r.Context(), project, change)
		respond(w, 200, out, err)
	}))
}
