package main

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"forgeos/internal/store"
)

func registerMaterials(mux *http.ServeMux, data *store.Store, owner string, workers *workerRegistry) {
	registerIndexedSearch(mux, "materials", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		q := r.URL.Query()
		kind, query, after := q.Get("kind"), strings.TrimSpace(q.Get("q")), q.Get("after")
		if (kind != "" && !store.ValidMaterialKind(kind)) || len(query) > 512 || !utf8.ValidString(query) || strings.ContainsRune(query, 0) ||
			(after != "" && (query != "" || !store.ValidMaterialKey(after))) {
			problem(w, 400, "invalid material kind, query or browse cursor")
			return
		}
		out, err := data.SearchMaterials(r.Context(), id, kind, query, after)
		respond(w, 200, out, err)
	})
	registerProjectRead(mux, "materials/{key}", owner, workers, func(w http.ResponseWriter, r *http.Request, id int64) {
		key := r.PathValue("key")
		if !store.ValidMaterialKey(key) {
			problem(w, 400, "invalid material key")
			return
		}
		var revision int64
		if r.URL.Query().Has("revision") {
			var err error
			revision, err = strconv.ParseInt(r.URL.Query().Get("revision"), 10, 32)
			if err != nil || revision < 1 {
				problem(w, 400, "invalid material revision")
				return
			}
		}
		out, err := data.Material(r.Context(), id, key, int(revision))
		respond(w, 200, out, err)
	})
	mux.HandleFunc("PUT /projects/{id}/materials/{key}", authorized(owner, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		key := r.PathValue("key")
		if !store.ValidMaterialKey(key) {
			problem(w, 400, "invalid material key")
			return
		}
		var m store.MaterialInput
		if !decodeLimited(w, r, &m, 2<<20) {
			return
		}
		if !m.Valid() {
			problem(w, 400, "provide a valid revision, kind, title, source and up to 256 KiB of text")
			return
		}
		out, err := data.SaveMaterial(r.Context(), id, key, m)
		respond(w, 200, out, err)
	}))
}
