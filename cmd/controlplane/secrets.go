package main

import (
	"net/http"

	"forgeos/internal/bao"
	"forgeos/internal/store"
)

func registerSecrets(mux *http.ServeMux, data *store.Store, owner string, workers workerAccess, vault *bao.Client) {
	const path = "/projects/{id}/environments/{environment}/secrets/{name}"
	target := func(w http.ResponseWriter, r *http.Request) (store.SecretMeta, bool) {
		id, ok := pathID(w, r)
		m := store.SecretMeta{ProjectID: id, Environment: r.PathValue("environment"), Name: r.PathValue("name")}
		if !ok {
			return m, false
		}
		if !store.ValidEnvironment(m.Environment) || !store.ValidSecretName(m.Name) {
			problem(w, 400, "invalid environment or secret name")
			return m, false
		}
		return m, true
	}
	mux.HandleFunc("PUT "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		m, ok := target(w, r)
		if !ok {
			return
		}
		var body struct {
			Risk string `json:"risk"`
		}
		if !decode(w, r, &body) {
			return
		}
		if !store.ValidRisk(body.Risk) {
			problem(w, 400, "choose a risk level from S0 through S3")
			return
		}
		m.Risk = body.Risk
		out, err := data.SetSecretMeta(r.Context(), m)
		respond(w, 200, out, err)
	}))
	read := func(w http.ResponseWriter, r *http.Request, m store.SecretMeta) {
		meta, err := data.SecretMeta(r.Context(), m.ProjectID, m.Environment, m.Name)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		status, err := vault.Status(r.Context(), meta)
		if err != nil {
			problem(w, 503, "OpenBao metadata is unavailable")
			return
		}
		writeJSON(w, 200, struct {
			store.SecretMeta
			bao.Status
		}{meta, status})
	}
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		if m, ok := target(w, r); ok {
			read(w, r, m)
		}
	}))
	mux.HandleFunc("GET /internal"+path, workers.authorize(func(w http.ResponseWriter, r *http.Request, g *workerGrant) {
		m, ok := target(w, r)
		if !ok {
			return
		}
		if !g.environments[environmentGrant{m.ProjectID, m.Environment}] {
			problem(w, 403, "environment is outside this Worker's grant")
			return
		}
		read(w, r, m)
	}))
}
