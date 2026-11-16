package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"forgeos/internal/store"
	"forgeos/internal/validation"
)

func registerCIFiles(mux *http.ServeMux, data *store.Store, owner string, workers workerAccess) {
	serve := func(w http.ResponseWriter, r *http.Request) {
		task, err := strconv.ParseInt(r.PathValue("task"), 10, 64)
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err != nil || task <= 0 {
			problem(w, 400, "invalid task ID")
			return
		}
		run, file := r.PathValue("run"), r.PathValue("file")
		want, err := data.CIFile(r.Context(), task, id, run, file)
		if err != nil {
			respond(w, 0, nil, err)
			return
		}
		key := fmt.Sprintf("ci/%d/%d/%s/%s", task, id, run, file)
		if r.Method == http.MethodPut {
			content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, want.Bytes))
			if err != nil || validation.DigestBytes(content) != want {
				problem(w, 422, "CI file does not match the stored receipt")
				return
			}
			if err := saveArtifact(key, content); err != nil {
				problem(w, 503, "CI artifact storage unavailable")
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		path, err := artifactPath(key)
		if err != nil {
			problem(w, 503, "CI artifact storage unavailable")
			return
		}
		f, err := os.Open(path)
		if err != nil {
			problem(w, 503, "CI 文件尚未归档或存储不可用")
			return
		}
		defer f.Close()
		content, err := io.ReadAll(io.LimitReader(f, want.Bytes+1))
		if err != nil || validation.DigestBytes(content) != want {
			problem(w, 503, "CI 文件未通过完整性校验")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, file))
		w.Header().Set("Content-Length", strconv.FormatInt(want.Bytes, 10))
		w.Write(content)
	}
	const path = "/tasks/{task}/executions/{id}/ci/{run}/{file}"
	mux.HandleFunc("GET "+path, authorized(owner, serve))
	mux.HandleFunc("PUT /internal"+path, workers.execution(data, serve))
}
