package main

import (
	"context"
	"database/sql"
	"net/http"
	"runtime"
	"time"

	"forgeos/internal/localrepo"
	"forgeos/internal/runtimeinfo"
)

func registerRuntimeStatus(ctx context.Context, mux *http.ServeMux, db *sql.DB, root, owner string) {
	monitor := runtimeinfo.Start(ctx, runtimeinfo.Paths{Repositories: root, Managed: localrepo.ManagedRoot()}, nil)
	mux.HandleFunc("GET /workers/control-plane", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		postgres := "reachable"
		if err := db.PingContext(ctx); err != nil {
			postgres = "unreachable"
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, struct {
			OS          string               `json:"os"`
			Arch        string               `json:"arch"`
			CPUs        int                  `json:"cpus"`
			PostgreSQL  string               `json:"postgresql"`
			Diagnostics runtimeinfo.Snapshot `json:"diagnostics"`
		}{runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), postgres, monitor.Snapshot()})
	}))
}
