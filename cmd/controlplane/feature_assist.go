package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"forgeos/internal/store"
	"github.com/google/uuid"
)

func registerFeatureAssist(mux *http.ServeMux, data *store.Store, owner string, nodes chatNodes) {
	base := "/projects/{id}/features/{feature}"

	mux.HandleFunc("POST "+base+"/notes/extract", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		if _, ready := nodes[project]; !ready {
			problem(w, http.StatusServiceUnavailable, "请先为此项目连接讨论节点。")
			return
		}
		feature, ok := requestedFeature(w, r, data, project)
		if !ok {
			return
		}
		var in struct {
			Client  uuid.UUID `json:"client_id"`
			Profile uuid.UUID `json:"profile_id"`
			Model   string    `json:"model"`
			Turn    int64     `json:"turn_id"`
		}
		if !decodeLimited(w, r, &in, 8192) {
			return
		}
		in.Model = strings.TrimSpace(in.Model)
		if in.Client == uuid.Nil || in.Profile == uuid.Nil || in.Turn < 0 || len(in.Model) > 160 || strings.ContainsRune(in.Model, 0) {
			problem(w, http.StatusBadRequest, "请选择有效的讨论模型。")
			return
		}
		job, err := data.StartNoteExtraction(r.Context(), project, feature, in.Turn, in.Client, in.Profile, in.Model)
		if errors.Is(err, store.ErrConflict) {
			problem(w, 409, "暂无可梳理的已完成讨论，或所选模型已失效。")
			return
		}
		respond(w, http.StatusCreated, job, err)
	}))

	mux.HandleFunc("GET "+base+"/notes/extract/{job}/status", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		feature, ok := requestedFeature(w, r, data, project)
		if !ok {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("job"), 10, 64)
		if err != nil || id < 1 {
			problem(w, http.StatusBadRequest, "无效的梳理任务。")
			return
		}
		job, err := data.FeatureAssistJob(r.Context(), project, feature, id)
		respond(w, http.StatusOK, job, err)
	}))

	mux.HandleFunc("POST "+base+"/notes/batch", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		feature, ok := requestedFeature(w, r, data, project)
		if !ok {
			return
		}
		var in store.BatchFeatureNotesInput
		if !decodeLimited(w, r, &in, 64<<10) {
			return
		}
		if (len(in.Items) == 0 && len(in.CloseIDs) == 0) || len(in.Items) > 16 || len(in.CloseIDs) > 32 {
			problem(w, http.StatusBadRequest, "请至少勾选一条记录进行操作。")
			return
		}
		saved, closed, err := data.BatchFeatureNotes(r.Context(), project, feature, in)
		if errors.Is(err, store.ErrConflict) {
			problem(w, 409, "部分记录来源无效或内容超出限制，请核对后重试。")
			return
		}
		respond(w, http.StatusOK, map[string]int{"saved": saved, "closed": closed}, err)
	}))

	mux.HandleFunc("GET "+base+"/brief", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		feature, ok := requestedFeature(w, r, data, project)
		if !ok {
			return
		}
		state, err := data.FeatureBrief(r.Context(), project, feature)
		respond(w, http.StatusOK, state, err)
	}))

	mux.HandleFunc("PUT "+base+"/brief", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		feature, ok := requestedFeature(w, r, data, project)
		if !ok {
			return
		}
		var in struct {
			Brief string `json:"brief"`
		}
		if !decodeLimited(w, r, &in, 32<<10) {
			return
		}
		if strings.ContainsRune(in.Brief, 0) || utf8.RuneCountInString(strings.TrimSpace(in.Brief)) > 4000 {
			problem(w, http.StatusBadRequest, "本轮需求终稿不能超过 4000 字。")
			return
		}
		state, err := data.SaveFeatureBrief(r.Context(), project, feature, in.Brief)
		respond(w, http.StatusOK, state, err)
	}))

	mux.HandleFunc("POST "+base+"/brief/generate", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		project, ok := pathID(w, r)
		if !ok {
			return
		}
		if _, ready := nodes[project]; !ready {
			problem(w, http.StatusServiceUnavailable, "请先为此项目连接讨论节点。")
			return
		}
		feature, ok := requestedFeature(w, r, data, project)
		if !ok {
			return
		}
		var in struct {
			Client  uuid.UUID `json:"client_id"`
			Profile uuid.UUID `json:"profile_id"`
			Model   string    `json:"model"`
		}
		if !decodeLimited(w, r, &in, 8192) {
			return
		}
		in.Model = strings.TrimSpace(in.Model)
		if in.Client == uuid.Nil || in.Profile == uuid.Nil || len(in.Model) > 160 || strings.ContainsRune(in.Model, 0) {
			problem(w, http.StatusBadRequest, "请选择有效的讨论模型。")
			return
		}
		job, err := data.StartFeatureBriefGeneration(r.Context(), project, feature, in.Client, in.Profile, in.Model)
		if errors.Is(err, store.ErrConflict) {
			problem(w, 409, "请先完成至少一轮讨论或添加话题记录，或检查所选模型是否可用。")
			return
		}
		respond(w, http.StatusCreated, job, err)
	}))
}
