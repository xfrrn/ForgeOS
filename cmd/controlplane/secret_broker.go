package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"forgeos/internal/bao"
	"forgeos/internal/store"
)

func registerSecretBroker(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry, metadata, broker *bao.Client) {
	const collection = "/projects/{id}/secret-grants"
	mux.HandleFunc("POST "+collection, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		var g store.SecretGrantInput
		if !decode(w, r, &g) {
			return
		}
		if !g.Valid() {
			problem(w, 400, "specify a worker execution, versioned key, purpose, expiry and request key")
			return
		}
		allowed := false
		for _, node := range workers.snapshot() {
			if node.id == g.Principal && node.environments[environmentGrant{project, g.Environment}] {
				allowed = true
				break
			}
		}
		if !allowed || g.Environment == "production" {
			problem(w, 403, "environment is outside the permitted broker scope")
			return
		}
		m, err := data.SecretMeta(r.Context(), project, g.Environment, g.Name)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		if m.Risk == "S3" {
			problem(w, 403, "this secret requires a privileged operation")
			return
		}
		if broker == nil {
			problem(w, 503, "OpenBao broker is unavailable")
			return
		}
		status, err := metadata.Status(r.Context(), m)
		if err != nil {
			problem(w, 503, "OpenBao metadata is unavailable")
			return
		}
		if status.State != "AVAILABLE" || status.Version != g.Version {
			respond(w, 0, nil, store.ErrConflict)
			return
		}
		doc, err := data.ApproveSecretGrant(r.Context(), m, g)
		respond(w, 200, doc, err)
	}))
	ids := func(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
		project, ok := pathID(w, r)
		if !ok {
			return 0, 0, false
		}
		id, err := strconv.ParseInt(r.PathValue("grant"), 10, 64)
		if err != nil || id <= 0 {
			problem(w, 400, "invalid secret grant id")
			return 0, 0, false
		}
		return project, id, true
	}
	mux.HandleFunc("GET "+collection+"/{grant}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, id, ok := ids(w, r)
		if !ok {
			return
		}
		doc, err := data.SecretGrantReceipt(r.Context(), project, id)
		respond(w, 200, doc, err)
	}))
	mux.HandleFunc("POST "+collection+"/{grant}/revoke", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, id, ok := ids(w, r)
		if !ok {
			return
		}
		respond(w, 204, nil, data.RevokeSecretGrant(r.Context(), project, id))
	}))
	mux.HandleFunc("POST /internal"+collection+"/{grant}/use", workers.authorize(func(w http.ResponseWriter, r *http.Request, node *workerGrant) {
		w.Header().Set("Cache-Control", "no-store")
		project, id, ok := ids(w, r)
		if !ok {
			return
		}
		var u store.SecretUse
		if !decode(w, r, &u) {
			return
		}
		if node.id == "" || !node.environments[environmentGrant{project, u.Environment}] {
			problem(w, 403, "environment is outside this Worker's grant")
			return
		}
		if broker == nil {
			problem(w, 503, "OpenBao broker is unavailable")
			return
		}
		var wrapped bao.Wrapped
		err := data.UseSecret(r.Context(), project, id, node.id, u, func(m store.SecretMeta, version int, until time.Time) (time.Time, error) {
			st, err := metadata.Status(r.Context(), m)
			if err != nil {
				return time.Time{}, err
			}
			if st.State != "AVAILABLE" || st.Version != version {
				return time.Time{}, store.ErrConflict
			}
			wrapped, err = broker.Wrap(r.Context(), m, version, until)
			return wrapped.ExpiresAt, err
		})
		if errors.Is(err, bao.ErrUnavailable) {
			problem(w, 503, "OpenBao broker is unavailable")
			return
		}
		respond(w, 200, wrapped, err)
	}))
}
