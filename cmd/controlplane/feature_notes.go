package main

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"forgeos/internal/store"
	"github.com/google/uuid"
)

func registerFeatureNotes(mux *http.ServeMux, data *store.Store, owner string) {
	const path = "/projects/{id}/features/{feature}/notes"
	handle := func(method, suffix string, run func(http.ResponseWriter, *http.Request, int64, int64, int64)) {
		mux.HandleFunc(method+" "+path+suffix, authorized(owner, func(w http.ResponseWriter, r *http.Request) {
			project, ok := pathID(w, r)
			if !ok {
				return
			}
			feature, ok := requestedFeature(w, r, data, project)
			if !ok {
				return
			}
			var id int64
			if suffix != "" {
				var err error
				id, err = strconv.ParseInt(r.PathValue("note"), 10, 64)
				if err != nil || id < 1 {
					problem(w, 400, "无效的话题记录")
					return
				}
			}
			run(w, r, project, feature, id)
		}))
	}
	handle("GET", "", func(w http.ResponseWriter, r *http.Request, project, feature, _ int64) {
		notes, err := data.FeatureNotes(r.Context(), project, feature, false)
		respond(w, 200, notes, err)
	})
	handle("POST", "", func(w http.ResponseWriter, r *http.Request, project, feature, _ int64) {
		var in struct {
			Turn   int64     `json:"turn_id"`
			Client uuid.UUID `json:"client_id"`
			Kind   string    `json:"kind"`
			Body   string    `json:"body"`
		}
		if !decodeLimited(w, r, &in, 16<<10) {
			return
		}
		in.Body = strings.TrimSpace(in.Body)
		if in.Turn <= 0 || in.Client == uuid.Nil || (in.Kind != "agreement" && in.Kind != "question") || in.Body == "" || strings.ContainsRune(in.Body, 0) || utf8.RuneCountInString(in.Body) > 1200 {
			problem(w, 400, "请选择讨论来源、记录类型，并填写 1 至 1200 字的内容。")
			return
		}
		note, err := data.AddFeatureNote(r.Context(), project, feature, in.Client, store.FeatureNote{Turn: in.Turn, Body: in.Body, Kind: in.Kind})
		respond(w, 201, note, err)
	})
	handle("GET", "/{note}/source", func(w http.ResponseWriter, r *http.Request, project, feature, note int64) {
		source, err := data.FeatureNoteSource(r.Context(), project, feature, note)
		respond(w, 200, source, err)
	})
	handle("POST", "/{note}/close", func(w http.ResponseWriter, r *http.Request, project, feature, note int64) {
		respond(w, 204, nil, data.CloseFeatureNote(r.Context(), project, feature, note))
	})
}
