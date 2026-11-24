package main

import (
	"forgeos/internal/store"
	"github.com/google/uuid"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

func requestedFeature(w http.ResponseWriter, r *http.Request, data *store.Store, project int64) (int64, bool) {
	raw := r.PathValue("feature")
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		problem(w, 400, "无效的功能话题")
		return 0, false
	}
	if _, err = data.Feature(r.Context(), project, id); err != nil {
		respond(w, 0, nil, err)
		return 0, false
	}
	return id, true
}

func registerFeatureTopics(mux *http.ServeMux, data *store.Store, owner string) {
	registerFeatureNotes(mux, data, owner)
	mux.HandleFunc("GET /projects/{id}/features", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		if _, err := data.Project(r.Context(), project); err != nil {
			respond(w, 0, nil, err)
			return
		}
		topics, err := data.Features(r.Context(), project)
		respond(w, 200, topics, err)
	}))
	mux.HandleFunc("POST /projects/{id}/features", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		var in struct {
			Title   string    `json:"title"`
			Client  uuid.UUID `json:"client_id"`
			Profile uuid.UUID `json:"profile_id"`
		}
		if !decodeLimited(w, r, &in, 4096) {
			return
		}
		in.Title = strings.TrimSpace(in.Title)
		if in.Client == uuid.Nil || in.Profile == uuid.Nil || in.Title == "" || strings.ContainsRune(in.Title, 0) || utf8.RuneCountInString(in.Title) > 120 {
			problem(w, 400, "请输入功能名称并选择讨论模型。")
			return
		}
		topic, err := data.CreateFeature(r.Context(), project, in.Client, in.Profile, in.Title)
		respond(w, 201, topic, err)
	}))
}

func registerFeatureClients(mux *http.ServeMux, data *store.Store, owner string, nodes chatNodes) {
	registerFeatureOperations(mux, data, owner, nodes)
	mux.HandleFunc("GET /projects/{id}/features/clients", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		list, err := data.FeatureClients(r.Context(), project)
		respond(w, 200, list, err)
	}))
	mux.HandleFunc("POST /internal/management-chat/feature-clients", nodes.authorize(func(project int64, w http.ResponseWriter, r *http.Request) {
		var in struct {
			Node         string
			Resource     string `json:"resource_worker"`
			ConfigFailed bool   `json:"config_failed"`
			Generation   int64
			Clients      []store.FeatureClient
		}
		if !decodeLimited(w, r, &in, 64<<10) {
			return
		}
		if in.Node == "" || len(in.Node) > 128 || len(in.Clients) > 32 || len(in.Resource) > 128 || in.Generation < 0 {
			problem(w, 400, "invalid client report")
			return
		}
		for _, c := range in.Clients {
			if !validProbeStatus(c.Status) || (c.Backend != "claude" && c.Backend != "codex") || c.Instance == "" || len(c.Instance) > 128 || c.Model == "" || len(c.Model) > 160 || len(c.AvailableModels) > 64 {
				problem(w, 400, "invalid client identity")
				return
			}
			for _, m := range c.AvailableModels {
				if m.ID == "" || len(m.ID) > 160 || len(m.Label) > 200 || strings.ContainsRune(m.ID, 0) || strings.ContainsRune(m.Label, 0) {
					problem(w, 400, "invalid client model option")
					return
				}
			}
		}
		respond(w, 204, nil, data.RecordFeatureProbe(r.Context(), project, in.Node, in.Resource, in.Clients, in.Generation, in.ConfigFailed))
	}))
}
