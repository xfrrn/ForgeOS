package main

import (
	"forgeos/internal/store"
	"github.com/google/uuid"
	"net/http"
	"slices"
	"strconv"
)

func validProbeStatus(s string) bool {
	return slices.Contains([]string{"", "ready", "auth", "unreachable", "timeout", "disabled", "not_installed", "configuration"}, s)
}
func registerFeatureOperations(mux *http.ServeMux, data *store.Store, owner string, nodes chatNodes) {
	const catalog = "/projects/{id}/features/clients"
	mux.HandleFunc("GET "+catalog+"/status", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		clients, err := data.ClientHealth(r.Context(), project)
		respond(w, 200, clients, err)
	}))
	mux.HandleFunc("POST "+catalog+"/{client}/recheck", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		id, err := uuid.Parse(r.PathValue("client"))
		if err != nil {
			problem(w, 400, "无效的客户端")
			return
		}
		respond(w, 204, nil, data.RequestFeatureProbe(r.Context(), project, id))
	}))
	mux.HandleFunc("POST /internal/management-chat/probe-request", nodes.authorize(func(project int64, w http.ResponseWriter, r *http.Request) {
		var in struct{ Node string }
		if !decodeLimited(w, r, &in, 1024) {
			return
		}
		gen, err := data.PendingFeatureProbe(r.Context(), project, in.Node)
		respond(w, 200, gen, err)
	}))
	mux.HandleFunc("POST /internal/management-chat/feature-turn-control", nodes.authorize(func(project int64, w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID    int64
			Lease uuid.UUID
		}
		if !decodeLimited(w, r, &in, 1024) {
			return
		}
		stop, err := data.FeatureCancelRequested(r.Context(), project, in.ID, in.Lease.String())
		respond(w, 200, stop, err)
	}))
	const turns = "POST /projects/{id}/features/{feature}/messages/{turn}/"
	for _, operation := range []string{"cancel", "recover"} {
		mux.HandleFunc(turns+operation, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			project, ok := pathID(w, r)
			if !ok {
				return
			}
			feature, ok := requestedFeature(w, r, data, project)
			if !ok {
				return
			}
			id, err := strconv.ParseInt(r.PathValue("turn"), 10, 64)
			if err != nil || id < 1 {
				problem(w, 400, "无效的讨论回合")
				return
			}
			if operation == "cancel" {
				respond(w, 204, nil, data.CancelFeatureTurn(r.Context(), project, feature, id))
				return
			}
			turn, err := data.RecoverFeatureTurn(r.Context(), project, feature, id)
			respond(w, 201, turn, err)
		}))
	}
}
