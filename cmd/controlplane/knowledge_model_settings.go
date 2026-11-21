package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"forgeos/internal/store"
	"github.com/google/uuid"
)

func registerKnowledgeModelSettings(mux *http.ServeMux, data *store.Store, owner string) {
	const path = "/projects/{id}/knowledge/model-settings"
	mux.HandleFunc("GET "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		settings, err := data.KnowledgeModelSettings(r.Context(), project)
		respond(w, http.StatusOK, settings, err)
	}))
	mux.HandleFunc("PUT "+path, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		var in struct {
			Role     string          `json:"role"`
			Expected *int64          `json:"expected_revision"`
			Model    json.RawMessage `json:"model_id"`
		}
		if !decodeLimited(w, r, &in, 2048) {
			return
		}
		var model *uuid.UUID
		if (in.Role != "curator" && in.Role != "management") || in.Expected == nil || *in.Expected < 0 ||
			json.Unmarshal(in.Model, &model) != nil || (model != nil && *model == uuid.Nil) {
			problem(w, http.StatusBadRequest, "请提供知识角色、预期配置版本和模型编号。停用时将模型编号设为 null。")
			return
		}
		settings, err := data.SetKnowledgeModel(r.Context(), project, in.Role, *in.Expected, model)
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "角色配置或模型目录已更改，请刷新再保存。")
			return
		}
		respond(w, http.StatusOK, settings, err)
	}))
}
